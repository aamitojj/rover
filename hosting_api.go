package main

// Provider API clients for hosting accounts: list servers (all providers),
// plans and create (Hetzner, DigitalOcean, Vultr, Linode, Lightsail), plus
// the two request-signing schemes (AWS SigV4, Oracle Cloud HTTP signatures).

import (
	"bytes"
	"context"
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// HostedServer is a server as the provider reports it.
type HostedServer struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	IP       string  `json:"ip"`
	Status   string  `json:"status"`
	Plan     string  `json:"plan,omitempty"`
	Region   string  `json:"region,omitempty"`
	CPUs     float64 `json:"cpus,omitempty"`
	MemoryGB float64 `json:"memory_gb,omitempty"`
	DiskGB   float64 `json:"disk_gb,omitempty"`
	User     string  `json:"user,omitempty"`
}

// API base addresses (variables so tests can point them at fakes).
var hostBase = map[string]string{
	"vultr":        "https://api.vultr.com/v2",
	"linode":       "https://api.linode.com/v4",
	"hostinger":    "https://developers.hostinger.com",
	"scaleway":     "https://api.scaleway.com",
	"upcloud":      "https://api.upcloud.com",
	"contabo":      "https://api.contabo.com",
	"contabo-auth": "https://auth.contabo.com/auth/realms/contabo/protocol/openid-connect/token",
	"lightsail":    "", // https://lightsail.<region>.amazonaws.com
	"oracle":       "", // https://iaas.<region>.oraclecloud.com
}

var hostHTTP = &http.Client{Timeout: 40 * time.Second}

type hostErr struct {
	Status int
	Msg    string
}

func (e *hostErr) Error() string { return e.Msg }

// hostDo performs one JSON request and decodes the answer into out.
func hostDo(ctx context.Context, provider, method, u string, hdr map[string]string, body []byte, out any) error {
	ctx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Rover/"+Version)
	if body != nil && hdr["Content-Type"] == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := hostHTTP.Do(req)
	if err != nil {
		return &hostErr{Msg: fmt.Sprintf("couldn't reach %s. Check your internet connection", provider)}
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode/100 != 2 {
		detail := providerErrorText(b)
		if detail == "" {
			detail = http.StatusText(resp.StatusCode)
		}
		if len(detail) > 200 {
			detail = detail[:200]
		}
		switch resp.StatusCode {
		case 401, 403:
			return &hostErr{resp.StatusCode, fmt.Sprintf("%s did not accept these details (%s). Check that you copied all of it and that it has the right access", provider, detail)}
		case 429:
			return &hostErr{429, provider + " says: too many requests. Wait a minute and try again"}
		}
		return &hostErr{resp.StatusCode, fmt.Sprintf("%s: %s", provider, detail)}
	}
	if out != nil && len(bytes.TrimSpace(b)) > 0 {
		if err := json.Unmarshal(b, out); err != nil {
			return fmt.Errorf("%s sent an answer Rover could not read", provider)
		}
	}
	return nil
}

func bearer(t string) map[string]string { return map[string]string{"Authorization": "Bearer " + t} }

func jsonBody(v any) []byte { b, _ := json.Marshal(v); return b }

// ---------- listing ----------

// listHostedServers asks the provider for the account's servers. who is a
// display name for the account when the API offers one.
func listHostedServers(ctx context.Context, prov string, cr map[string]string) (servers []HostedServer, who string, err error) {
	p, _ := hostingProviderByID(prov)
	switch prov {
	case "hetzner", "digitalocean":
		cp := cloudProvider{kind: prov, token: cr["token"]}
		return cp.ListServers(ctx)
	case "vultr":
		var v struct {
			Instances []struct {
				ID, Label, MainIP, Status, PowerStatus, Plan, Region string
				VCPU                                                 float64 `json:"vcpu_count"`
				RAM, Disk                                            float64
			} `json:"instances"`
		}
		if err := hostDo(ctx, p.Name, "GET", hostBase["vultr"]+"/instances?per_page=100", bearer(cr["token"]), nil, &vultrFix{&v}); err != nil {
			return nil, "", err
		}
		for _, i := range v.Instances {
			servers = append(servers, HostedServer{ID: i.ID, Name: i.Label, IP: zeroIP(i.MainIP), Status: firstNonEmpty(i.PowerStatus, i.Status), Plan: i.Plan, Region: i.Region, CPUs: i.VCPU, MemoryGB: i.RAM / 1024, DiskGB: i.Disk, User: p.User})
		}
		var a struct {
			Account struct{ Email, Name string } `json:"account"`
		}
		if hostDo(ctx, p.Name, "GET", hostBase["vultr"]+"/account", bearer(cr["token"]), nil, &a) == nil {
			who = firstNonEmpty(a.Account.Email, a.Account.Name)
		}
		return servers, who, nil
	case "linode":
		var v struct {
			Data []struct {
				ID     int64    `json:"id"`
				Label  string   `json:"label"`
				Status string   `json:"status"`
				IPv4   []string `json:"ipv4"`
				Type   string   `json:"type"`
				Region string   `json:"region"`
				Specs  struct {
					VCPUs  float64 `json:"vcpus"`
					Memory float64 `json:"memory"`
					Disk   float64 `json:"disk"`
				} `json:"specs"`
			} `json:"data"`
		}
		if err := hostDo(ctx, p.Name, "GET", hostBase["linode"]+"/linode/instances?page_size=100", bearer(cr["token"]), nil, &v); err != nil {
			return nil, "", err
		}
		for _, i := range v.Data {
			servers = append(servers, HostedServer{ID: strconv.FormatInt(i.ID, 10), Name: i.Label, IP: publicIPv4(i.IPv4), Status: i.Status, Plan: i.Type, Region: i.Region, CPUs: i.Specs.VCPUs, MemoryGB: i.Specs.Memory / 1024, DiskGB: i.Specs.Disk / 1024, User: p.User})
		}
		var pr struct{ Username, Email string }
		if hostDo(ctx, p.Name, "GET", hostBase["linode"]+"/profile", bearer(cr["token"]), nil, &pr) == nil {
			who = firstNonEmpty(pr.Email, pr.Username)
		}
		return servers, who, nil
	case "hostinger":
		var v []struct {
			ID       int64   `json:"id"`
			Hostname string  `json:"hostname"`
			State    string  `json:"state"`
			Plan     string  `json:"plan"`
			CPUs     float64 `json:"cpus"`
			Memory   float64 `json:"memory"`
			Disk     float64 `json:"disk"`
			IPv4     []struct {
				Address string `json:"address"`
			} `json:"ipv4"`
		}
		if err := hostDo(ctx, p.Name, "GET", hostBase["hostinger"]+"/api/vps/v1/virtual-machines", bearer(cr["token"]), nil, &v); err != nil {
			return nil, "", err
		}
		for _, i := range v {
			ip := ""
			if len(i.IPv4) > 0 {
				ip = i.IPv4[0].Address
			}
			servers = append(servers, HostedServer{ID: strconv.FormatInt(i.ID, 10), Name: i.Hostname, IP: ip, Status: i.State, Plan: i.Plan, CPUs: i.CPUs, MemoryGB: i.Memory / 1024, DiskGB: i.Disk / 1024, User: p.User})
		}
		return servers, "", nil
	case "scaleway":
		zones := []string{"fr-par-1", "fr-par-2", "fr-par-3", "nl-ams-1", "nl-ams-2", "nl-ams-3", "pl-waw-1", "pl-waw-2", "pl-waw-3"}
		hdr := map[string]string{"X-Auth-Token": cr["token"]}
		okZones := 0
		var lastErr error
		for _, z := range zones {
			var v struct {
				Servers []struct {
					ID, Name, State string
					CommercialType  string `json:"commercial_type"`
					PublicIP        *struct {
						Address string `json:"address"`
					} `json:"public_ip"`
					PublicIPs []struct {
						Address string `json:"address"`
						Family  string `json:"family"`
					} `json:"public_ips"`
				} `json:"servers"`
			}
			if err := hostDo(ctx, p.Name, "GET", hostBase["scaleway"]+"/instance/v1/zones/"+z+"/servers?per_page=50", hdr, nil, &v); err != nil {
				var he *hostErr
				if errors.As(err, &he) && (he.Status == 401) {
					return nil, "", err
				}
				lastErr = err
				continue
			}
			okZones++
			for _, s := range v.Servers {
				ip := ""
				if s.PublicIP != nil {
					ip = s.PublicIP.Address
				}
				for _, x := range s.PublicIPs {
					if ip == "" && x.Family != "inet6" {
						ip = x.Address
					}
				}
				servers = append(servers, HostedServer{ID: z + "/" + s.ID, Name: s.Name, IP: ip, Status: s.State, Plan: s.CommercialType, Region: z, User: p.User})
			}
		}
		if okZones == 0 && lastErr != nil {
			return nil, "", lastErr
		}
		return servers, "", nil
	case "upcloud":
		var v struct {
			Servers struct {
				Server []struct {
					UUID, Title, Hostname, State, Plan, Zone string
					Cores                                    string `json:"core_number"`
					Memory                                   string `json:"memory_amount"`
				} `json:"server"`
			} `json:"servers"`
		}
		if err := hostDo(ctx, p.Name, "GET", hostBase["upcloud"]+"/1.3/server", bearer(cr["token"]), nil, &v); err != nil {
			return nil, "", err
		}
		for i, s := range v.Servers.Server {
			hs := HostedServer{ID: s.UUID, Name: firstNonEmpty(s.Title, s.Hostname), Status: s.State, Plan: s.Plan, Region: s.Zone, User: p.User}
			hs.CPUs, _ = strconv.ParseFloat(s.Cores, 64)
			if m, err := strconv.ParseFloat(s.Memory, 64); err == nil {
				hs.MemoryGB = m / 1024
			}
			if i < 25 { // the list has no addresses; ask per server
				var d struct {
					Server struct {
						IPAddresses struct {
							IPAddress []struct{ Access, Family, Address string } `json:"ip_address"`
						} `json:"ip_addresses"`
					} `json:"server"`
				}
				if hostDo(ctx, p.Name, "GET", hostBase["upcloud"]+"/1.3/server/"+url.PathEscape(s.UUID), bearer(cr["token"]), nil, &d) == nil {
					for _, a := range d.Server.IPAddresses.IPAddress {
						if a.Access == "public" && a.Family == "IPv4" {
							hs.IP = a.Address
							break
						}
					}
				}
			}
			servers = append(servers, hs)
		}
		var a struct {
			Account struct{ Username string } `json:"account"`
		}
		if hostDo(ctx, p.Name, "GET", hostBase["upcloud"]+"/1.3/account", bearer(cr["token"]), nil, &a) == nil {
			who = a.Account.Username
		}
		return servers, who, nil
	case "contabo":
		tok, err := contaboToken(ctx, cr)
		if err != nil {
			return nil, "", err
		}
		var v struct {
			Data []struct {
				InstanceID  int64   `json:"instanceId"`
				Name        string  `json:"name"`
				DisplayName string  `json:"displayName"`
				Status      string  `json:"status"`
				ProductName string  `json:"productName"`
				Region      string  `json:"region"`
				CPUCores    float64 `json:"cpuCores"`
				RAMMb       float64 `json:"ramMb"`
				DiskMb      float64 `json:"diskMb"`
				IPConfig    struct {
					V4 struct {
						IP string `json:"ip"`
					} `json:"v4"`
				} `json:"ipConfig"`
			} `json:"data"`
		}
		h := bearer(tok)
		h["x-request-id"] = uuid4()
		if err := hostDo(ctx, p.Name, "GET", hostBase["contabo"]+"/v1/compute/instances?size=100", h, nil, &v); err != nil {
			return nil, "", err
		}
		for _, i := range v.Data {
			servers = append(servers, HostedServer{ID: strconv.FormatInt(i.InstanceID, 10), Name: firstNonEmpty(i.DisplayName, i.Name), IP: i.IPConfig.V4.IP, Status: i.Status, Plan: i.ProductName, Region: i.Region, CPUs: i.CPUCores, MemoryGB: i.RAMMb / 1024, DiskGB: i.DiskMb / 1024, User: p.User})
		}
		return servers, cr["user"], nil
	case "lightsail":
		var v struct {
			Instances []struct {
				Name            string                `json:"name"`
				PublicIPAddress string                `json:"publicIpAddress"`
				BundleID        string                `json:"bundleId"`
				Username        string                `json:"username"`
				State           struct{ Name string } `json:"state"`
				Location        struct {
					RegionName string `json:"regionName"`
				} `json:"location"`
				Hardware struct {
					CPUCount    float64 `json:"cpuCount"`
					RAMSizeInGb float64 `json:"ramSizeInGb"`
				} `json:"hardware"`
			} `json:"instances"`
		}
		if err := lightsailCall(ctx, cr, "GetInstances", map[string]any{}, &v); err != nil {
			return nil, "", err
		}
		for _, i := range v.Instances {
			servers = append(servers, HostedServer{ID: i.Name, Name: i.Name, IP: i.PublicIPAddress, Status: i.State.Name, Plan: i.BundleID, Region: i.Location.RegionName, CPUs: i.Hardware.CPUCount, MemoryGB: i.Hardware.RAMSizeInGb, User: firstNonEmpty(i.Username, p.User)})
		}
		return servers, cr["region"], nil
	case "oracle":
		return ociListServers(ctx, cr)
	}
	return nil, "", errors.New("this provider has no server list; use Any other VPS")
}

// vultrFix maps Vultr's snake_case fields onto the anonymous struct above.
type vultrFix struct{ v any }

func (f *vultrFix) UnmarshalJSON(b []byte) error {
	var raw struct {
		Instances []map[string]any `json:"instances"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	for _, m := range raw.Instances {
		for k, nk := range map[string]string{"main_ip": "MainIP", "power_status": "PowerStatus", "vcpu_count": "vcpu_count"} {
			if x, ok := m[k]; ok {
				m[nk] = x
			}
		}
	}
	nb, _ := json.Marshal(raw)
	return json.Unmarshal(nb, f.v)
}

func zeroIP(ip string) string {
	if ip == "0.0.0.0" {
		return ""
	}
	return ip
}

func publicIPv4(ips []string) string {
	for _, ip := range ips {
		if !strings.HasPrefix(ip, "192.168.") && !strings.HasPrefix(ip, "10.") && !strings.Contains(ip, ":") {
			return ip
		}
	}
	return ""
}

func uuid4() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	h := hex.EncodeToString(b)
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

func contaboToken(ctx context.Context, cr map[string]string) (string, error) {
	form := url.Values{"client_id": {cr["client_id"]}, "client_secret": {cr["client_secret"]}, "username": {cr["user"]}, "password": {cr["password"]}, "grant_type": {"password"}}
	var t struct {
		AccessToken string `json:"access_token"`
	}
	if err := hostDo(ctx, "Contabo", "POST", hostBase["contabo-auth"], map[string]string{"Content-Type": "application/x-www-form-urlencoded"}, []byte(form.Encode()), &t); err != nil {
		return "", err
	}
	if t.AccessToken == "" {
		return "", errors.New("Contabo did not return a login token; check the four values")
	}
	return t.AccessToken, nil
}

// ListServers lists Hetzner servers or DigitalOcean droplets.
func (p cloudProvider) ListServers(ctx context.Context) ([]HostedServer, string, error) {
	var out []HostedServer
	if p.kind == "hetzner" {
		var v struct {
			Servers []struct {
				ID        int64  `json:"id"`
				Name      string `json:"name"`
				Status    string `json:"status"`
				PublicNet struct {
					IPv4 *struct{ IP string } `json:"ipv4"`
				} `json:"public_net"`
				ServerType struct {
					Name   string  `json:"name"`
					Cores  float64 `json:"cores"`
					Memory float64 `json:"memory"`
					Disk   float64 `json:"disk"`
				} `json:"server_type"`
				Datacenter struct {
					Location struct{ Name string } `json:"location"`
				} `json:"datacenter"`
			} `json:"servers"`
		}
		if err := p.do(ctx, "GET", "/servers?per_page=50", nil, &v); err != nil {
			return nil, "", err
		}
		for _, s := range v.Servers {
			ip := ""
			if s.PublicNet.IPv4 != nil {
				ip = s.PublicNet.IPv4.IP
			}
			out = append(out, HostedServer{ID: strconv.FormatInt(s.ID, 10), Name: s.Name, IP: ip, Status: s.Status, Plan: s.ServerType.Name, Region: s.Datacenter.Location.Name, CPUs: s.ServerType.Cores, MemoryGB: s.ServerType.Memory, DiskGB: s.ServerType.Disk, User: "root"})
		}
		return out, "", nil
	}
	var v struct {
		Droplets []struct {
			ID       int64                 `json:"id"`
			Name     string                `json:"name"`
			Status   string                `json:"status"`
			SizeSlug string                `json:"size_slug"`
			VCPUs    float64               `json:"vcpus"`
			Memory   float64               `json:"memory"`
			Disk     float64               `json:"disk"`
			Region   struct{ Slug string } `json:"region"`
			Networks struct {
				V4 []struct {
					IPAddress string `json:"ip_address"`
					Type      string `json:"type"`
				} `json:"v4"`
			} `json:"networks"`
		} `json:"droplets"`
	}
	if err := p.do(ctx, "GET", "/droplets?per_page=100", nil, &v); err != nil {
		return nil, "", err
	}
	for _, d := range v.Droplets {
		hs := HostedServer{ID: strconv.FormatInt(d.ID, 10), Name: d.Name, Status: d.Status, Plan: d.SizeSlug, Region: d.Region.Slug, CPUs: d.VCPUs, MemoryGB: d.Memory / 1024, DiskGB: d.Disk, User: "root"}
		for _, n := range d.Networks.V4 {
			if n.Type == "public" {
				hs.IP = n.IPAddress
			}
		}
		out = append(out, hs)
	}
	var a struct {
		Account struct{ Email string } `json:"account"`
	}
	who := ""
	if p.do(ctx, "GET", "/account", nil, &a) == nil {
		who = a.Account.Email
	}
	return out, who, nil
}

// ---------- plans and create ----------

func hostingPlans(ctx context.Context, prov string, cr map[string]string) ([]CloudPlan, error) {
	switch prov {
	case "hetzner", "digitalocean":
		return cloudProvider{kind: prov, token: cr["token"]}.Plans(ctx)
	case "vultr":
		var v struct {
			Plans []struct {
				ID          string   `json:"id"`
				VCPU        float64  `json:"vcpu_count"`
				RAM         float64  `json:"ram"`
				Disk        float64  `json:"disk"`
				MonthlyCost float64  `json:"monthly_cost"`
				Locations   []string `json:"locations"`
			} `json:"plans"`
		}
		if err := hostDo(ctx, "Vultr", "GET", hostBase["vultr"]+"/plans?type=vc2&per_page=500", bearer(cr["token"]), nil, &v); err != nil {
			return nil, err
		}
		var out []CloudPlan
		for _, p := range v.Plans {
			if p.VCPU >= 2 && p.RAM >= 2048 && p.Disk >= 30 && len(p.Locations) > 0 {
				out = append(out, CloudPlan{ID: p.ID, Label: p.ID, Cores: p.VCPU, MemoryGB: p.RAM / 1024, DiskGB: p.Disk, Monthly: p.MonthlyCost, Currency: "USD", Regions: p.Locations})
			}
		}
		return cheapest(out), nil
	case "linode":
		var v struct {
			Data []struct {
				ID     string  `json:"id"`
				Label  string  `json:"label"`
				VCPUs  float64 `json:"vcpus"`
				Memory float64 `json:"memory"`
				Disk   float64 `json:"disk"`
				Class  string  `json:"class"`
				Price  struct {
					Monthly float64 `json:"monthly"`
				} `json:"price"`
			} `json:"data"`
		}
		if err := hostDo(ctx, "Linode", "GET", hostBase["linode"]+"/linode/types", bearer(cr["token"]), nil, &v); err != nil {
			return nil, err
		}
		var rg struct {
			Data []struct {
				ID           string   `json:"id"`
				Status       string   `json:"status"`
				Capabilities []string `json:"capabilities"`
			} `json:"data"`
		}
		var regions []string
		if hostDo(ctx, "Linode", "GET", hostBase["linode"]+"/regions", bearer(cr["token"]), nil, &rg) == nil {
			for _, r := range rg.Data {
				if r.Status == "ok" && contains(r.Capabilities, "Linodes") {
					regions = append(regions, r.ID)
				}
			}
		}
		if len(regions) == 0 {
			regions = []string{"in-maa", "ap-south", "ap-west", "eu-central", "us-east"}
		}
		var out []CloudPlan
		for _, t := range v.Data {
			if (t.Class == "standard" || t.Class == "nanode") && t.VCPUs >= 2 && t.Memory >= 2048 {
				out = append(out, CloudPlan{ID: t.ID, Label: firstNonEmpty(t.Label, t.ID), Cores: t.VCPUs, MemoryGB: t.Memory / 1024, DiskGB: t.Disk / 1024, Monthly: t.Price.Monthly, Currency: "USD", Regions: regions})
			}
		}
		return cheapest(out), nil
	case "lightsail":
		var v struct {
			Bundles []struct {
				BundleID           string   `json:"bundleId"`
				Name               string   `json:"name"`
				Price              float64  `json:"price"`
				CPUCount           float64  `json:"cpuCount"`
				RAMSizeInGb        float64  `json:"ramSizeInGb"`
				DiskSizeInGb       float64  `json:"diskSizeInGb"`
				IsActive           bool     `json:"isActive"`
				SupportedPlatforms []string `json:"supportedPlatforms"`
			} `json:"bundles"`
		}
		if err := lightsailCall(ctx, cr, "GetBundles", map[string]any{"includeInactive": false}, &v); err != nil {
			return nil, err
		}
		var out []CloudPlan
		for _, b := range v.Bundles {
			if b.IsActive && contains(b.SupportedPlatforms, "LINUX_UNIX") && b.CPUCount >= 2 && b.RAMSizeInGb >= 2 && !strings.Contains(b.BundleID, "ipv6") {
				out = append(out, CloudPlan{ID: b.BundleID, Label: firstNonEmpty(b.Name, b.BundleID), Cores: b.CPUCount, MemoryGB: b.RAMSizeInGb, DiskGB: b.DiskSizeInGb, Monthly: b.Price, Currency: "USD", Regions: []string{cr["region"]}})
			}
		}
		return cheapest(out), nil
	}
	return nil, errors.New("Rover cannot create servers at this provider. Order one on their website, then click Refresh")
}

func cheapest(p []CloudPlan) []CloudPlan {
	sort.Slice(p, func(i, j int) bool { return p[i].Monthly < p[j].Monthly })
	if len(p) > 6 {
		p = p[:6]
	}
	return p
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

// createHostedServer creates a server with Rover's public key and waits
// for its public IPv4. user is the SSH login of the new server.
func createHostedServer(ctx context.Context, j *Job, prov string, cr map[string]string, name, plan, region, pub string) (id, ip, user string, err error) {
	switch prov {
	case "hetzner", "digitalocean":
		id, ip, err = cloudProvider{kind: prov, token: cr["token"]}.CreateServer(ctx, j, name, plan, region, pub)
		return id, ip, "root", err
	case "vultr":
		h := bearer(cr["token"])
		var osl struct {
			OS []struct {
				ID   int64  `json:"id"`
				Name string `json:"name"`
				Arch string `json:"arch"`
			} `json:"os"`
		}
		if err := hostDo(ctx, "Vultr", "GET", hostBase["vultr"]+"/os?per_page=500", h, nil, &osl); err != nil {
			return "", "", "", err
		}
		var osID int64
		for _, o := range osl.OS {
			if strings.Contains(o.Name, "Ubuntu 24.04") && o.Arch == "x64" {
				osID = o.ID
				break
			}
		}
		if osID == 0 {
			return "", "", "", errors.New("Vultr has no Ubuntu 24.04 image right now")
		}
		var k struct {
			SSHKey struct {
				ID string `json:"id"`
			} `json:"ssh_key"`
		}
		if err := hostDo(ctx, "Vultr", "POST", hostBase["vultr"]+"/ssh-keys", h, jsonBody(map[string]any{"name": "rover-" + randHex(3), "ssh_key": pub}), &k); err != nil {
			return "", "", "", err
		}
		var in struct {
			Instance struct {
				ID string `json:"id"`
			} `json:"instance"`
		}
		if err := hostDo(ctx, "Vultr", "POST", hostBase["vultr"]+"/instances", h, jsonBody(map[string]any{"region": region, "plan": plan, "os_id": osID, "label": name, "hostname": name, "sshkey_id": []string{k.SSHKey.ID}, "tags": []string{"rover"}}), &in); err != nil {
			return "", "", "", err
		}
		id = in.Instance.ID
		ip, err = waitIP(ctx, j, "Vultr", func() (string, bool, error) {
			var v struct {
				Instance map[string]any `json:"instance"`
			}
			if err := hostDo(ctx, "Vultr", "GET", hostBase["vultr"]+"/instances/"+url.PathEscape(id), h, nil, &v); err != nil {
				return "", false, err
			}
			ip, _ := v.Instance["main_ip"].(string)
			st, _ := v.Instance["status"].(string)
			ps, _ := v.Instance["power_status"].(string)
			return zeroIP(ip), st == "active" && ps == "running", nil
		})
		return id, ip, "root", err
	case "linode":
		h := bearer(cr["token"])
		var in struct {
			ID   int64    `json:"id"`
			IPv4 []string `json:"ipv4"`
		}
		body := map[string]any{"type": plan, "region": region, "image": "linode/ubuntu24.04", "label": name, "root_pass": strongPassword(), "authorized_keys": []string{strings.TrimSpace(pub)}, "booted": true, "tags": []string{"rover"}}
		if err := hostDo(ctx, "Linode", "POST", hostBase["linode"]+"/linode/instances", h, jsonBody(body), &in); err != nil {
			return "", "", "", err
		}
		id = strconv.FormatInt(in.ID, 10)
		ip, err = waitIP(ctx, j, "Linode", func() (string, bool, error) {
			var v struct {
				Status string   `json:"status"`
				IPv4   []string `json:"ipv4"`
			}
			if err := hostDo(ctx, "Linode", "GET", hostBase["linode"]+"/linode/instances/"+id, h, nil, &v); err != nil {
				return "", false, err
			}
			return publicIPv4(v.IPv4), v.Status == "running", nil
		})
		return id, ip, "root", err
	case "lightsail":
		region := cr["region"]
		script := "#!/bin/bash\nmkdir -p /home/ubuntu/.ssh\necho '" + strings.TrimSpace(pub) + "' >> /home/ubuntu/.ssh/authorized_keys\nchown -R ubuntu:ubuntu /home/ubuntu/.ssh\nchmod 600 /home/ubuntu/.ssh/authorized_keys\n"
		if err := lightsailCall(ctx, cr, "CreateInstances", map[string]any{"instanceNames": []string{name}, "availabilityZone": region + "a", "blueprintId": "ubuntu_24_04", "bundleId": plan, "userData": script, "tags": []map[string]string{{"key": "managed-by", "value": "rover"}}}, nil); err != nil {
			return "", "", "", err
		}
		id = name
		ip, err = waitIP(ctx, j, "AWS Lightsail", func() (string, bool, error) {
			var v struct {
				Instance struct {
					PublicIPAddress string                `json:"publicIpAddress"`
					State           struct{ Name string } `json:"state"`
				} `json:"instance"`
			}
			if err := lightsailCall(ctx, cr, "GetInstance", map[string]any{"instanceName": name}, &v); err != nil {
				return "", false, err
			}
			return v.Instance.PublicIPAddress, v.Instance.State.Name == "running", nil
		})
		if err == nil { // Lightsail's firewall only opens 22 and 80 by default
			for _, pr := range [][2]int{{443, 443}, {8000, 8000}, {6001, 6002}} {
				if e := lightsailCall(ctx, cr, "OpenInstancePublicPorts", map[string]any{"instanceName": name, "portInfo": map[string]any{"fromPort": pr[0], "toPort": pr[1], "protocol": "tcp"}}, nil); e != nil {
					j.Logf("Could not open port %d in the Lightsail firewall (%v). Open it in the Lightsail console > Networking.", pr[0], e)
				}
			}
		}
		return id, ip, "ubuntu", err
	}
	return "", "", "", errors.New("Rover cannot create servers at this provider")
}

func waitIP(ctx context.Context, j *Job, label string, poll func() (string, bool, error)) (string, error) {
	j.Logf("%s is starting the server...", label)
	deadline := time.Now().Add(8 * time.Minute)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(hostPollEvery):
		}
		ip, ready, err := poll()
		if err == nil && ready && ip != "" {
			return ip, nil
		}
	}
	return "", errors.New("the server did not start within 8 minutes; check the provider's website")
}

var hostPollEvery = 5 * time.Second

func strongPassword() string {
	return "Dl-" + base64.RawURLEncoding.EncodeToString([]byte(randHex(18))) + "#9"
}

// ---------- AWS Signature Version 4 (Lightsail) ----------

var awsNow = func() time.Time { return time.Now().UTC() }

func lightsailCall(ctx context.Context, cr map[string]string, op string, in, out any) error {
	region := strings.TrimSpace(cr["region"])
	if region == "" {
		return errors.New("choose the Lightsail region")
	}
	endpoint := hostBase["lightsail"]
	if endpoint == "" {
		endpoint = "https://lightsail." + region + ".amazonaws.com"
	}
	body := jsonBody(in)
	u, _ := url.Parse(endpoint + "/")
	hdr := map[string]string{"Content-Type": "application/x-amz-json-1.1", "X-Amz-Target": "Lightsail_20161128." + op}
	sigV4(hdr, "POST", u, body, cr["access_key"], cr["secret_key"], region, "lightsail", awsNow())
	return hostDo(ctx, "AWS Lightsail", "POST", u.String(), hdr, body, out)
}

func hmacSHA256(key []byte, s string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(s))
	return m.Sum(nil)
}

// sigV4 adds X-Amz-Date and Authorization headers (AWS Signature Version 4).
func sigV4(hdr map[string]string, method string, u *url.URL, body []byte, akid, secretKey, region, service string, now time.Time) {
	amzDate := now.Format("20060102T150405Z")
	day := now.Format("20060102")
	hdr["X-Amz-Date"] = amzDate
	host := u.Host
	ph := sha256.Sum256(body)
	payloadHash := hex.EncodeToString(ph[:])
	all := map[string]string{"host": host}
	for k, v := range hdr {
		all[strings.ToLower(k)] = strings.TrimSpace(v)
	}
	var names []string
	for k := range all {
		names = append(names, k)
	}
	sort.Strings(names)
	var ch strings.Builder
	for _, k := range names {
		ch.WriteString(k + ":" + all[k] + "\n")
	}
	signed := strings.Join(names, ";")
	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}
	q := u.Query()
	var qs []string
	for k, vs := range q {
		for _, v := range vs {
			qs = append(qs, awsEscape(k)+"="+awsEscape(v))
		}
	}
	sort.Strings(qs)
	canonical := strings.Join([]string{method, path, strings.Join(qs, "&"), ch.String(), signed, payloadHash}, "\n")
	ch2 := sha256.Sum256([]byte(canonical))
	scope := day + "/" + region + "/" + service + "/aws4_request"
	sts := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + hex.EncodeToString(ch2[:])
	k := hmacSHA256([]byte("AWS4"+secretKey), day)
	k = hmacSHA256(k, region)
	k = hmacSHA256(k, service)
	k = hmacSHA256(k, "aws4_request")
	sig := hex.EncodeToString(hmacSHA256(k, sts))
	hdr["Authorization"] = "AWS4-HMAC-SHA256 Credential=" + akid + "/" + scope + ", SignedHeaders=" + signed + ", Signature=" + sig
}

func awsEscape(s string) string {
	return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
}

// ---------- Oracle Cloud request signing ----------

func parseRSAKey(pemText string) (*rsa.PrivateKey, error) {
	blk, _ := pem.Decode([]byte(strings.TrimSpace(pemText)))
	if blk == nil {
		return nil, errors.New("the private key is not a .pem key; paste the whole file including the BEGIN and END lines")
	}
	if k, err := x509.ParsePKCS1PrivateKey(blk.Bytes); err == nil {
		return k, nil
	}
	k, err := x509.ParsePKCS8PrivateKey(blk.Bytes)
	if err != nil {
		return nil, errors.New("could not read the private key (keys with a passphrase are not supported)")
	}
	rk, ok := k.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("Oracle API keys are RSA keys; this is a different kind of key")
	}
	return rk, nil
}

// ociSign adds date, host and an Oracle "Signature" Authorization header.
func ociSign(hdr map[string]string, method string, u *url.URL, cr map[string]string, now time.Time) error {
	key, err := parseRSAKey(cr["key"])
	if err != nil {
		return err
	}
	date := now.UTC().Format(http.TimeFormat)
	hdr["Date"] = date
	target := strings.ToLower(method) + " " + u.RequestURI()
	signing := "(request-target): " + target + "\ndate: " + date + "\nhost: " + u.Host
	h := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, h[:])
	if err != nil {
		return err
	}
	keyID := strings.TrimSpace(cr["tenancy"]) + "/" + strings.TrimSpace(cr["user"]) + "/" + strings.TrimSpace(cr["fingerprint"])
	hdr["Authorization"] = fmt.Sprintf(`Signature version="1",keyId="%s",algorithm="rsa-sha256",headers="(request-target) date host",signature="%s"`, keyID, base64.StdEncoding.EncodeToString(sig))
	return nil
}

func ociGet(ctx context.Context, cr map[string]string, rawURL string, out any) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	hdr := map[string]string{}
	if err := ociSign(hdr, "GET", u, cr, time.Now()); err != nil {
		return err
	}
	return hostDo(ctx, "Oracle Cloud", "GET", u.String(), hdr, nil, out)
}

func ociListServers(ctx context.Context, cr map[string]string) ([]HostedServer, string, error) {
	region := strings.TrimSpace(cr["region"])
	base := hostBase["oracle"]
	if base == "" {
		base = "https://iaas." + region + ".oraclecloud.com"
	}
	comp := url.QueryEscape(strings.TrimSpace(cr["tenancy"]))
	var list []struct {
		ID             string `json:"id"`
		DisplayName    string `json:"displayName"`
		LifecycleState string `json:"lifecycleState"`
		Shape          string `json:"shape"`
		ShapeConfig    struct {
			OCPUs       float64 `json:"ocpus"`
			MemoryInGBs float64 `json:"memoryInGBs"`
		} `json:"shapeConfig"`
	}
	if err := ociGet(ctx, cr, base+"/20160918/instances?compartmentId="+comp+"&limit=100", &list); err != nil {
		return nil, "", err
	}
	var out []HostedServer
	for _, i := range list {
		if i.LifecycleState == "TERMINATED" {
			continue
		}
		hs := HostedServer{ID: i.ID, Name: i.DisplayName, Status: strings.ToLower(i.LifecycleState), Plan: i.Shape, Region: region, CPUs: i.ShapeConfig.OCPUs, MemoryGB: i.ShapeConfig.MemoryInGBs, User: "ubuntu"}
		var att []struct {
			VnicID string `json:"vnicId"`
		}
		if ociGet(ctx, cr, base+"/20160918/vnicAttachments?compartmentId="+comp+"&instanceId="+url.QueryEscape(i.ID), &att) == nil {
			for _, a := range att {
				var vn struct {
					PublicIP string `json:"publicIp"`
				}
				if ociGet(ctx, cr, base+"/20160918/vnics/"+url.PathEscape(a.VnicID), &vn) == nil && vn.PublicIP != "" {
					hs.IP = vn.PublicIP
					break
				}
			}
		}
		out = append(out, hs)
	}
	return out, region, nil
}

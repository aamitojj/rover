package prov

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"net/url"
	"strings"
	"time"
)

func init() {
	Register(ociProv{})
	Register(awsProv{})
	Register(gcpProv{})
	Register(azureProv{})
}

type ociProv struct{}

func (ociProv) Meta() Meta {
	return Meta{
		ID: "oracle", Name: "Oracle Cloud", Auth: "keys",
		Docs: "https://docs.oracle.com/en-us/iaas/Content/API/Concepts/signingrequests.htm",
		Capabilities: map[Capability]Support{
			CapHosting:    SupportPartial, // list instances; Rover does not create paid shapes
			CapCost:       SupportPartial, // usage API if subscribed; otherwise unsupported at Test time
			CapMonitoring: SupportPartial,
		},
		Fields: []Field{
			{Key: "tenancy", Label: "Tenancy OCID", Placeholder: "ocid1.tenancy.oc1..."},
			{Key: "user", Label: "User OCID", Placeholder: "ocid1.user.oc1..."},
			{Key: "fingerprint", Label: "Fingerprint"},
			{Key: "region", Label: "Region", Placeholder: "ap-mumbai-1"},
			{Key: "key", Label: "Private key (.pem)", Secret: true, Multiline: true},
		},
		Scopes: []string{"An API signing key whose user can inspect instances (INSTANCE_INSPECT) in the tenancy root or a compartment."},
		Notes:  "Lists compute instances with signed OCI requests. Rover never creates a paid shape; Always Free creation stays in the Oracle console.",
	}
}

func (ociProv) Test(ctx context.Context, tr *Transport, creds map[string]string) (*Info, error) {
	if err := require(creds, "tenancy", "user", "fingerprint", "region", "key"); err != nil {
		return nil, err
	}
	region := strings.TrimSpace(creds["region"])
	base := tr.BaseURL("oracle", "https://iaas."+region+".oraclecloud.com")
	u, err := url.Parse(base + "/20160918/instances?compartmentId=" + url.QueryEscape(strings.TrimSpace(creds["tenancy"])) + "&limit=100")
	if err != nil {
		return nil, err
	}
	hdr := map[string]string{}
	if err := ociSign(hdr, "GET", u, creds["tenancy"], creds["user"], creds["fingerprint"], creds["key"], time.Now()); err != nil {
		return nil, err
	}
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
	if err := tr.DoJSON(ctx, "GET", u.String(), hdr, nil, &list); err != nil {
		return nil, err
	}
	info := &Info{Who: creds["tenancy"], Health: "ok", Status: StatusConnected, Usage: map[string]any{"instances": len(list)}}
	for _, i := range list {
		if i.LifecycleState == "TERMINATED" {
			continue
		}
		info.Resources = append(info.Resources, Resource{ID: i.ID, Name: i.DisplayName, Kind: "instance", Region: region, Status: strings.ToLower(i.LifecycleState), Extra: i.Shape})
	}
	return info, nil
}

type awsProv struct{}

func (awsProv) Meta() Meta {
	return Meta{
		ID: "aws", Name: "AWS", Auth: "keys",
		Docs: "https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_aws-signing.html",
		Capabilities: map[Capability]Support{
			CapHosting:    SupportPartial, // Lightsail + EC2 describe only
			CapStorage:    SupportYes,     // S3 list
			CapCost:       SupportYes,     // Cost Explorer read — PAID per request
			CapMonitoring: SupportPartial,
		},
		Fields: []Field{
			{Key: "access_key", Label: "Access key ID", Placeholder: "AKIA…"},
			{Key: "secret_key", Label: "Secret access key", Secret: true},
			{Key: "region", Label: "Region", Options: []string{"ap-south-1", "us-east-1", "us-east-2", "us-west-2", "eu-west-1", "eu-west-2", "eu-central-1", "ap-southeast-1", "ap-northeast-1", "ap-southeast-2", "ca-central-1"}},
		},
		Scopes: []string{
			"IAM user or role with: lightsail:Get*, ec2:DescribeInstances, s3:ListAllMyBuckets, ce:GetCostAndUsage (optional).",
			"Do not attach AdministratorAccess.",
		},
		PaidWarning: "AWS Cost Explorer charges about $0.01 per API request. Rover will not call it unless you confirm.",
		Notes:       "Read-only: Lightsail instances, EC2 DescribeInstances, S3 bucket list, optional Cost Explorer. Rover never starts paid AWS resources from here.",
	}
}

func (awsProv) Test(ctx context.Context, tr *Transport, creds map[string]string) (*Info, error) {
	if err := require(creds, "access_key", "secret_key", "region"); err != nil {
		return nil, err
	}
	info := &Info{Who: keyHint(creds["access_key"]), Health: "ok", Status: StatusConnected, Usage: map[string]any{}}
	var errs []string
	if n, res, err := awsLightsail(ctx, tr, creds); err != nil {
		errs = append(errs, "Lightsail: "+err.Error())
	} else {
		info.Usage["lightsail"] = n
		info.Resources = append(info.Resources, res...)
	}
	if n, res, err := awsEC2(ctx, tr, creds); err != nil {
		errs = append(errs, "EC2: "+err.Error())
	} else {
		info.Usage["ec2"] = n
		info.Resources = append(info.Resources, res...)
	}
	if n, res, err := awsS3(ctx, tr, creds); err != nil {
		errs = append(errs, "S3: "+err.Error())
	} else {
		info.Usage["s3_buckets"] = n
		info.Resources = append(info.Resources, res...)
	}
	if len(errs) == 3 {
		info.Status, info.Health = StatusFailed, "degraded"
		return info, fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	if len(errs) > 0 {
		info.Health = "degraded"
		info.Usage["warnings"] = errs
	}
	return info, nil
}

func (awsProv) Cost(ctx context.Context, tr *Transport, creds map[string]string, confirmPaid bool) (*CostReport, error) {
	if !confirmPaid {
		return nil, fmt.Errorf("AWS Cost Explorer is a paid API (about $0.01 per request). Confirm before Rover calls it")
	}
	if err := require(creds, "access_key", "secret_key"); err != nil {
		return nil, err
	}
	// Cost Explorer is always us-east-1.
	base := tr.BaseURL("aws-ce", "https://ce.us-east-1.amazonaws.com")
	u, _ := url.Parse(base + "/")
	end := time.Now().UTC().Format("2006-01-02")
	start := time.Now().UTC().AddDate(0, 0, -30).Format("2006-01-02")
	body := []byte(fmt.Sprintf(`{"TimePeriod":{"Start":"%s","End":"%s"},"Granularity":"MONTHLY","Metrics":["UnblendedCost"]}`, start, end))
	hdr := map[string]string{"Content-Type": "application/x-amz-json-1.1", "X-Amz-Target": "AWSInsightsIndexService.GetCostAndUsage"}
	sigV4(hdr, "POST", u, body, creds["access_key"], creds["secret_key"], "us-east-1", "ce", time.Now())
	var out struct {
		ResultsByTime []struct {
			Total map[string]struct {
				Amount string `json:"Amount"`
				Unit   string `json:"Unit"`
			} `json:"Total"`
		} `json:"ResultsByTime"`
	}
	if err := tr.Do(ctx, "POST", u.String(), hdr, bytes.NewReader(body), &out); err != nil {
		return nil, err
	}
	rep := &CostReport{Period: start + " … " + end, Note: "Unblended cost, last 30 days. Cost Explorer itself is billed by AWS."}
	if len(out.ResultsByTime) > 0 {
		if c, ok := out.ResultsByTime[len(out.ResultsByTime)-1].Total["UnblendedCost"]; ok {
			rep.Total, rep.Currency = c.Amount, c.Unit
		}
	}
	return rep, nil
}

func awsLightsail(ctx context.Context, tr *Transport, creds map[string]string) (int, []Resource, error) {
	region := creds["region"]
	base := tr.BaseURL("aws-lightsail", "https://lightsail."+region+".amazonaws.com")
	u, _ := url.Parse(base + "/")
	body := []byte(`{}`)
	hdr := map[string]string{"Content-Type": "application/x-amz-json-1.1", "X-Amz-Target": "Lightsail_20161128.GetInstances"}
	sigV4(hdr, "POST", u, body, creds["access_key"], creds["secret_key"], region, "lightsail", time.Now())
	var out struct {
		Instances []struct {
			Name  string `json:"name"`
			Arn   string `json:"arn"`
			State struct {
				Name string `json:"name"`
			} `json:"state"`
			PublicIPAddress string `json:"publicIpAddress"`
		} `json:"instances"`
	}
	if err := tr.Do(ctx, "POST", u.String(), hdr, bytes.NewReader(body), &out); err != nil {
		return 0, nil, err
	}
	var res []Resource
	for _, i := range out.Instances {
		res = append(res, Resource{ID: firstNonEmpty(i.Arn, i.Name), Name: i.Name, Kind: "lightsail", Region: region, Status: i.State.Name, URL: i.PublicIPAddress})
	}
	return len(res), res, nil
}

func awsEC2(ctx context.Context, tr *Transport, creds map[string]string) (int, []Resource, error) {
	region := creds["region"]
	base := tr.BaseURL("aws-ec2", "https://ec2."+region+".amazonaws.com")
	form := url.Values{"Action": {"DescribeInstances"}, "Version": {"2016-11-15"}}
	u, _ := url.Parse(base + "/")
	body := []byte(form.Encode())
	hdr := map[string]string{"Content-Type": "application/x-www-form-urlencoded"}
	sigV4(hdr, "POST", u, body, creds["access_key"], creds["secret_key"], region, "ec2", time.Now())
	var raw []byte
	if err := tr.Do(ctx, "POST", u.String(), hdr, bytes.NewReader(body), &raw); err != nil {
		return 0, nil, err
	}
	var doc struct {
		Reservations []struct {
			Instances []struct {
				InstanceId string `xml:"instanceId"`
				State      struct {
					Name string `xml:"name"`
				} `xml:"instanceState"`
				PublicIp string `xml:"ipAddress"`
				Tags     []struct {
					Key   string `xml:"key"`
					Value string `xml:"value"`
				} `xml:"tagSet>item"`
			} `xml:"instancesSet>item"`
		} `xml:"reservationSet>item"`
	}
	if err := xml.Unmarshal(raw, &doc); err != nil {
		return 0, nil, fmt.Errorf("EC2 reply was not XML: %w", err)
	}
	var res []Resource
	for _, r := range doc.Reservations {
		for _, i := range r.Instances {
			name := i.InstanceId
			for _, t := range i.Tags {
				if t.Key == "Name" && t.Value != "" {
					name = t.Value
				}
			}
			res = append(res, Resource{ID: i.InstanceId, Name: name, Kind: "ec2", Region: region, Status: i.State.Name, URL: i.PublicIp})
		}
	}
	return len(res), res, nil
}

func awsS3(ctx context.Context, tr *Transport, creds map[string]string) (int, []Resource, error) {
	region := creds["region"]
	base := tr.BaseURL("aws-s3", "https://s3."+region+".amazonaws.com")
	u, _ := url.Parse(base + "/")
	hdr := map[string]string{}
	sigV4(hdr, "GET", u, nil, creds["access_key"], creds["secret_key"], region, "s3", time.Now())
	var raw []byte
	if err := tr.Do(ctx, "GET", u.String(), hdr, nil, &raw); err != nil {
		return 0, nil, err
	}
	var doc struct {
		Buckets []struct {
			Name string `xml:"Name"`
		} `xml:"Buckets>Bucket"`
	}
	if err := xml.Unmarshal(raw, &doc); err != nil {
		return 0, nil, fmt.Errorf("S3 reply was not XML: %w", err)
	}
	var res []Resource
	for _, b := range doc.Buckets {
		res = append(res, Resource{ID: b.Name, Name: b.Name, Kind: "bucket", Region: region})
	}
	return len(res), res, nil
}

func keyHint(k string) string {
	k = strings.TrimSpace(k)
	if len(k) <= 8 {
		return "••••"
	}
	return k[:4] + "…" + k[len(k)-2:]
}

type gcpProv struct{}

func (gcpProv) Meta() Meta {
	return Meta{
		ID: "gcp", Name: "Google Cloud", Auth: "service_account",
		Docs: "https://cloud.google.com/compute/docs/reference/rest/v1",
		Capabilities: map[Capability]Support{
			CapHosting:    SupportPartial, // list Compute instances
			CapStorage:    SupportPartial, // Cloud Storage list (if scoped)
			CapCost:       SupportNo,      // Cloud Billing reports need a BigQuery export; not a simple API
			CapMonitoring: SupportPartial,
		},
		Fields: []Field{
			{Key: "json", Label: "Service account JSON", Secret: true, Multiline: true},
			{Key: "project_id", Label: "Project ID (if not in the JSON)", Optional: true},
		},
		Scopes: []string{"Service account with resourcemanager.projects.get, compute.instances.list, and optionally storage.buckets.list. No Billing Account Admin."},
		Notes:  "Cost is marked unsupported: Cloud Billing export lives in BigQuery and is not queried here. Rover never creates GCP resources from this screen.",
	}
}

func (gcpProv) Test(ctx context.Context, tr *Transport, creds map[string]string) (*Info, error) {
	hdr, err := googleBearer(ctx, tr, creds, "https://www.googleapis.com/auth/cloud-platform.read-only")
	if err != nil {
		return nil, err
	}
	project := firebaseProjectID(creds)
	if project == "" {
		return nil, missing("project_id")
	}
	crm := tr.BaseURL("gcp-crm", "https://cloudresourcemanager.googleapis.com")
	var prj struct {
		ProjectID      string `json:"projectId"`
		Name           string `json:"name"`
		LifecycleState string `json:"lifecycleState"`
	}
	if err := tr.DoJSON(ctx, "GET", crm+"/v1/projects/"+project, hdr, nil, &prj); err != nil {
		return nil, err
	}
	info := &Info{Who: firstNonEmpty(prj.Name, prj.ProjectID), Health: "ok", Status: StatusConnected, Plan: prj.LifecycleState}
	comp := tr.BaseURL("gcp-compute", "https://compute.googleapis.com")
	var agg struct {
		Items map[string]struct {
			Instances []struct {
				ID     string `json:"id"`
				Name   string `json:"name"`
				Status string `json:"status"`
				Zone   string `json:"zone"`
			} `json:"instances"`
		} `json:"items"`
	}
	if err := tr.DoJSON(ctx, "GET", comp+"/compute/v1/projects/"+project+"/aggregated/instances", hdr, nil, &agg); err == nil {
		n := 0
		for zone, it := range agg.Items {
			for _, i := range it.Instances {
				n++
				info.Resources = append(info.Resources, Resource{ID: i.ID, Name: i.Name, Kind: "instance", Region: zone, Status: i.Status})
			}
		}
		info.Usage = map[string]any{"instances": n}
	}
	return info, nil
}

type azureProv struct{}

func (azureProv) Meta() Meta {
	return Meta{
		ID: "azure", Name: "Microsoft Azure", Auth: "keys",
		Docs: "https://learn.microsoft.com/en-us/rest/api/azure/",
		Capabilities: map[Capability]Support{
			CapHosting:    SupportPartial, // list VMs
			CapCost:       SupportYes,     // Cost Management query (may require enrollment)
			CapMonitoring: SupportPartial,
		},
		Fields: []Field{
			{Key: "tenant_id", Label: "Tenant ID"},
			{Key: "client_id", Label: "Application (client) ID"},
			{Key: "client_secret", Label: "Client secret", Secret: true},
			{Key: "subscription_id", Label: "Subscription ID", Optional: true},
		},
		Scopes: []string{"An app registration with a client secret, granted Reader on the subscription (and Cost Management Reader to see spend). Never Contributor just for Rover."},
		Notes:  "Uses OAuth2 client credentials against login.microsoftonline.com and the Azure Resource Manager API. Rover never creates paid resources from this screen.",
	}
}

func (azureProv) Test(ctx context.Context, tr *Transport, creds map[string]string) (*Info, error) {
	tok, err := azureToken(ctx, tr, creds)
	if err != nil {
		return nil, err
	}
	arm := tr.BaseURL("azure-arm", "https://management.azure.com")
	hdr := bearer(tok)
	var subs struct {
		Value []struct {
			ID             string `json:"id"`
			SubscriptionID string `json:"subscriptionId"`
			DisplayName    string `json:"displayName"`
			State          string `json:"state"`
		} `json:"value"`
	}
	if err := tr.DoJSON(ctx, "GET", arm+"/subscriptions?api-version=2020-01-01", hdr, nil, &subs); err != nil {
		return nil, err
	}
	info := &Info{Health: "ok", Status: StatusConnected, Usage: map[string]any{"subscriptions": len(subs.Value)}}
	want := strings.TrimSpace(creds["subscription_id"])
	for _, s := range subs.Value {
		if want != "" && s.SubscriptionID != want {
			continue
		}
		info.Who = firstNonEmpty(s.DisplayName, s.SubscriptionID)
		info.Resources = append(info.Resources, Resource{ID: s.SubscriptionID, Name: s.DisplayName, Kind: "subscription", Status: s.State})
		var vms struct {
			Value []struct {
				ID         string `json:"id"`
				Name       string `json:"name"`
				Location   string `json:"location"`
				Properties struct {
					ProvisioningState string `json:"provisioningState"`
				} `json:"properties"`
			} `json:"value"`
		}
		if err := tr.DoJSON(ctx, "GET", arm+"/subscriptions/"+s.SubscriptionID+"/providers/Microsoft.Compute/virtualMachines?api-version=2024-07-01", hdr, nil, &vms); err == nil {
			for _, v := range vms.Value {
				info.Resources = append(info.Resources, Resource{ID: v.ID, Name: v.Name, Kind: "vm", Region: v.Location, Status: v.Properties.ProvisioningState})
			}
			info.Usage["vms"] = len(vms.Value)
		}
	}
	if info.Who == "" && len(subs.Value) > 0 {
		info.Who = subs.Value[0].DisplayName
	}
	return info, nil
}

func azureToken(ctx context.Context, tr *Transport, creds map[string]string) (string, error) {
	if t := strings.TrimSpace(creds["access_token"]); t != "" {
		return t, nil
	}
	if err := require(creds, "tenant_id", "client_id", "client_secret"); err != nil {
		return "", err
	}
	base := tr.BaseURL("azure-login", "https://login.microsoftonline.com")
	var tok struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error_description"`
	}
	form := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {creds["client_id"]},
		"client_secret": {creds["client_secret"]},
		"scope":         {"https://management.azure.com/.default"},
	}
	if err := tr.DoForm(ctx, base+"/"+strings.TrimSpace(creds["tenant_id"])+"/oauth2/v2.0/token", nil, form, &tok); err != nil {
		return "", err
	}
	if tok.AccessToken == "" {
		return "", fmt.Errorf("Azure did not return an access token: %s", tok.Error)
	}
	return tok.AccessToken, nil
}

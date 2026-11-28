package prov

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCatalogUniqueAndCapabilities(t *testing.T) {
	seen := map[string]bool{}
	for _, m := range Catalog() {
		if m.ID == "" || m.Name == "" || seen[m.ID] || len(m.Capabilities) == 0 {
			t.Fatalf("invalid or duplicate: %+v", m)
		}
		seen[m.ID] = true
	}
	need := []string{"coolify", "nginx", "local", "supabase", "firebase", "netlify", "vercel", "oracle", "aws", "gcp", "azure", "cloudflare", "github", "gitlab", "ga4", "search-console", "adsense", "rover-analytics"}
	for _, id := range need {
		if !seen[id] {
			t.Fatalf("missing provider %s", id)
		}
	}
	if !Supports("supabase", CapAuth) || !Supports("coolify", CapHosting) || Supports("supabase", CapHosting) || Supports("unknown", CapHosting) {
		t.Fatal("capability resolution")
	}
	if Supports("github", CapHosting) || !Supports("github", CapImport) {
		t.Fatal("github must be import-only")
	}
	if Supports("gcp", CapCost) {
		t.Fatal("gcp cost should be unsupported, not faked")
	}
	if _, ok := Lookup("coolify"); !ok {
		t.Fatal("lookup")
	}
}

func TestCostExplorerRequiresConfirmation(t *testing.T) {
	p, _ := Lookup("aws")
	coster, ok := p.(Coster)
	if !ok {
		t.Fatal("aws must implement Coster")
	}
	_, err := coster.Cost(context.Background(), &Transport{}, map[string]string{"access_key": "A", "secret_key": "B"}, false)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "confirm") {
		t.Fatalf("unconfirmed cost call must fail: %v", err)
	}
}

func muxJSON(t *testing.T, routes map[string]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + " " + r.URL.Path
		body, ok := routes[key]
		if !ok {
			body, ok = routes[r.URL.Path]
		}
		if !ok {
			http.Error(w, "no mock for "+key, 404)
			return
		}
		if strings.HasPrefix(body, "XML:") {
			w.Header().Set("Content-Type", "text/xml")
			io.WriteString(w, strings.TrimPrefix(body, "XML:"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, body)
	}))
}

func TestNetlifyAndVercelMocks(t *testing.T) {
	ctx := context.Background()
	ns := muxJSON(t, map[string]string{
		"GET /api/v1/user":  `{"id":"u1","email":"a@example.com","full_name":"Ada","site_count":1}`,
		"GET /api/v1/sites": `[{"id":"s1","name":"shop","url":"https://shop.netlify.app","state":"current","plan_name":"Starter"}]`,
	})
	defer ns.Close()
	p, _ := Lookup("netlify")
	info, err := p.Test(ctx, &Transport{Base: map[string]string{"netlify": ns.URL}}, map[string]string{"token": "nfp_secret_token_value"})
	if err != nil || info.Who != "Ada" || len(info.Resources) != 1 || info.Plan != "Starter" {
		t.Fatalf("netlify: %+v %v", info, err)
	}

	vs := muxJSON(t, map[string]string{
		"GET /v2/user":     `{"user":{"id":"1","username":"ada","name":"Ada","email":"a@example.com"}}`,
		"GET /v9/projects": `{"projects":[{"id":"p1","name":"shop","framework":"vite"}]}`,
	})
	defer vs.Close()
	vp, _ := Lookup("vercel")
	info, err = vp.Test(ctx, &Transport{Base: map[string]string{"vercel": vs.URL}}, map[string]string{"token": "vercel_secret"})
	if err != nil || info.Who != "Ada" || len(info.Resources) != 1 {
		t.Fatalf("vercel: %+v %v", info, err)
	}
}

func TestCoolifyAndSupabaseAndCloudflareMocks(t *testing.T) {
	ctx := context.Background()
	cs := muxJSON(t, map[string]string{
		"GET /api/v1/version":      `{"version":"4.0.0"}`,
		"GET /api/v1/applications": `[{"uuid":"a1","name":"shop","fqdn":"shop.example.com","status":"running"}]`,
	})
	defer cs.Close()
	cp, _ := Lookup("coolify")
	info, err := cp.Test(ctx, &Transport{Base: map[string]string{"coolify": cs.URL}}, map[string]string{"url": cs.URL, "token": "c-token"})
	if err != nil || !strings.Contains(info.Who, "4.0.0") || len(info.Resources) != 1 {
		t.Fatalf("coolify: %+v %v", info, err)
	}

	ss := muxJSON(t, map[string]string{
		"GET /v1/projects": `[{"id":"1","ref":"abcd","name":"app","region":"ap-south-1","status":"ACTIVE"}]`,
	})
	defer ss.Close()
	sp, _ := Lookup("supabase")
	info, err = sp.Test(ctx, &Transport{Base: map[string]string{"supabase": ss.URL}}, map[string]string{"token": "sbp_secret"})
	if err != nil || len(info.Resources) != 1 || info.Resources[0].ID != "abcd" {
		t.Fatalf("supabase: %+v %v", info, err)
	}

	cf := muxJSON(t, map[string]string{
		"GET /client/v4/user/tokens/verify": `{"success":true,"result":{"id":"t","status":"active"}}`,
		"GET /client/v4/zones":              `{"success":true,"result":[{"id":"z1","name":"example.com","status":"active","plan":{"name":"Free"}}]}`,
	})
	defer cf.Close()
	cfp, _ := Lookup("cloudflare")
	info, err = cfp.Test(ctx, &Transport{Base: map[string]string{"cloudflare": cf.URL}}, map[string]string{"token": "cf-token"})
	if err != nil || info.Plan != "Free" || len(info.Resources) != 1 {
		t.Fatalf("cloudflare: %+v %v", info, err)
	}
}

func TestGitHubGitLabGSCAdSenseMocks(t *testing.T) {
	ctx := context.Background()
	gh := muxJSON(t, map[string]string{
		"GET /user":       `{"login":"ada","name":"Ada"}`,
		"GET /user/repos": `[{"full_name":"ada/shop","html_url":"https://github.com/ada/shop","private":false}]`,
	})
	defer gh.Close()
	p, _ := Lookup("github")
	info, err := p.Test(ctx, &Transport{Base: map[string]string{"github": gh.URL}}, map[string]string{"token": "ghp_secret"})
	if err != nil || info.Who != "Ada" {
		t.Fatalf("github: %+v %v", info, err)
	}

	gl := muxJSON(t, map[string]string{
		"GET /api/v4/user":     `{"username":"ada","name":"Ada"}`,
		"GET /api/v4/projects": `[{"path_with_namespace":"ada/shop","web_url":"https://gitlab.com/ada/shop"}]`,
	})
	defer gl.Close()
	gp, _ := Lookup("gitlab")
	info, err = gp.Test(ctx, &Transport{Base: map[string]string{"gitlab": gl.URL}}, map[string]string{"token": "glpat-secret"})
	if err != nil || info.Who != "Ada" {
		t.Fatalf("gitlab: %+v %v", info, err)
	}

	gsc := muxJSON(t, map[string]string{
		"GET /webmasters/v3/sites": `{"siteEntry":[{"siteUrl":"https://example.com/","permissionLevel":"siteFullUser"}]}`,
	})
	defer gsc.Close()
	gscp, _ := Lookup("search-console")
	info, err = gscp.Test(ctx, &Transport{Base: map[string]string{"gsc": gsc.URL}}, map[string]string{"access_token": "ya29.secret"})
	if err != nil || len(info.Resources) != 1 {
		t.Fatalf("gsc: %+v %v", info, err)
	}

	ad := muxJSON(t, map[string]string{
		"GET /v2/accounts": `{"accounts":[{"name":"accounts/pub-1","displayName":"Ada","state":"READY"}]}`,
	})
	defer ad.Close()
	ap, _ := Lookup("adsense")
	info, err = ap.Test(ctx, &Transport{Base: map[string]string{"adsense": ad.URL}}, map[string]string{"access_token": "ya29.secret"})
	if err != nil || info.Who != "Ada" {
		t.Fatalf("adsense: %+v %v", info, err)
	}

	ga := muxJSON(t, map[string]string{
		"GET /v1beta/accountSummaries": `{"accountSummaries":[{"propertySummaries":[{"property":"properties/1","displayName":"Shop"}]}]}`,
	})
	defer ga.Close()
	gap, _ := Lookup("ga4")
	info, err = gap.Test(ctx, &Transport{Base: map[string]string{"ga-admin": ga.URL}}, map[string]string{"access_token": "ya29.secret"})
	if err != nil || info.Who != "Shop" {
		t.Fatalf("ga4: %+v %v", info, err)
	}
}

func TestAzureAndAWSMocks(t *testing.T) {
	ctx := context.Background()
	login := muxJSON(t, map[string]string{
		"POST /ten/oauth2/v2.0/token": `{"access_token":"aztok"}`,
	})
	defer login.Close()
	arm := muxJSON(t, map[string]string{
		"GET /subscriptions": `{"value":[{"subscriptionId":"sub1","displayName":"Pay-As-You-Go","state":"Enabled"}]}`,
		"GET /subscriptions/sub1/providers/Microsoft.Compute/virtualMachines": `{"value":[{"id":"/vm/1","name":"web","location":"eastus","properties":{"provisioningState":"Succeeded"}}]}`,
	})
	defer arm.Close()
	p, _ := Lookup("azure")
	info, err := p.Test(ctx, &Transport{Base: map[string]string{"azure-login": login.URL, "azure-arm": arm.URL}}, map[string]string{"tenant_id": "ten", "client_id": "id", "client_secret": "sec"})
	if err != nil || info.Who != "Pay-As-You-Go" || len(info.Resources) < 2 {
		t.Fatalf("azure: %+v %v", info, err)
	}

	aws := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" && r.URL.Path != "/" {
			http.Error(w, "unsigned", 403)
			return
		}
		target := r.Header.Get("X-Amz-Target")
		switch {
		case target == "Lightsail_20161128.GetInstances":
			io.WriteString(w, `{"instances":[{"name":"box","arn":"arn:aws:lightsail:1","state":{"name":"running"},"publicIpAddress":"1.2.3.4"}]}`)
		case strings.Contains(r.Header.Get("Content-Type"), "www-form") || strings.Contains(string(mustRead(r)), "DescribeInstances"):
			w.Header().Set("Content-Type", "text/xml")
			io.WriteString(w, `<DescribeInstancesResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/"><reservationSet><item><instancesSet><item><instanceId>i-1</instanceId><instanceState><name>running</name></instanceState><ipAddress>1.2.3.4</ipAddress><tagSet><item><key>Name</key><value>web</value></item></tagSet></item></instancesSet></item></reservationSet></DescribeInstancesResponse>`)
		default: // S3 list
			w.Header().Set("Content-Type", "text/xml")
			io.WriteString(w, `<?xml version="1.0"?><ListAllMyBucketsResult><Buckets><Bucket><Name>my-bucket</Name></Bucket></Buckets></ListAllMyBucketsResult>`)
		}
	}))
	defer aws.Close()
	ap, _ := Lookup("aws")
	info, err = ap.Test(ctx, &Transport{Base: map[string]string{"aws-lightsail": aws.URL, "aws-ec2": aws.URL, "aws-s3": aws.URL}}, map[string]string{"access_key": "AKIAEXAMPLE", "secret_key": "secret", "region": "ap-south-1"})
	if err != nil || info.Status != StatusConnected {
		t.Fatalf("aws: %+v %v", info, err)
	}
	kinds := map[string]int{}
	for _, r := range info.Resources {
		kinds[r.Kind]++
	}
	if kinds["lightsail"] != 1 || kinds["ec2"] != 1 || kinds["bucket"] != 1 {
		t.Fatalf("aws resources: %+v", info.Resources)
	}
}

func mustRead(r *http.Request) []byte {
	b, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(strings.NewReader(string(b)))
	return b
}

func TestOCISignedMock(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	der := x509.MarshalPKCS1PrivateKey(key)
	pemKey := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: der})
	var sawAuth bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Signature ") {
			http.Error(w, "unsigned", 401)
			return
		}
		sawAuth = true
		io.WriteString(w, `[{"id":"ocid1.instance","displayName":"free-ampere","lifecycleState":"RUNNING","shape":"VM.Standard.A1.Flex","shapeConfig":{"ocpus":1,"memoryInGBs":6}}]`)
	}))
	defer srv.Close()
	p, _ := Lookup("oracle")
	info, err := p.Test(context.Background(), &Transport{Base: map[string]string{"oracle": srv.URL}}, map[string]string{
		"tenancy": "ocid1.tenancy", "user": "ocid1.user", "fingerprint": "ab:cd", "region": "ap-mumbai-1", "key": string(pemKey),
	})
	if err != nil || !sawAuth || len(info.Resources) != 1 {
		t.Fatalf("oci: %+v %v auth=%v", info, err, sawAuth)
	}
}

func TestSigV4Stable(t *testing.T) {
	u, _ := http.NewRequest("GET", "https://s3.ap-south-1.amazonaws.com/", nil)
	hdr := map[string]string{}
	now := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	sigV4(hdr, "GET", u.URL, nil, "AKID", "SECRET", "ap-south-1", "s3", now)
	if !strings.Contains(hdr["Authorization"], "AWS4-HMAC-SHA256 Credential=AKID/20240102/ap-south-1/s3/aws4_request") {
		t.Fatalf("authorization: %s", hdr["Authorization"])
	}
	if hdr["X-Amz-Date"] != "20240102T030405Z" {
		t.Fatalf("date %s", hdr["X-Amz-Date"])
	}
}

func TestAPIErrorDoesNotEchoLongBodies(t *testing.T) {
	e := APIError{Status: 401, Body: strings.Repeat("secret-token-value-", 80)}
	if len(e.Error()) > 300 || !strings.Contains(e.Error(), "401") {
		t.Fatalf("error too long or missing status: %s", e.Error())
	}
}

func TestMissingCreds(t *testing.T) {
	for _, id := range []string{"netlify", "vercel", "supabase", "aws", "azure", "oracle", "cloudflare", "github"} {
		p, _ := Lookup(id)
		_, err := p.Test(context.Background(), &Transport{}, map[string]string{})
		if err == nil {
			t.Fatalf("%s accepted empty creds", id)
		}
	}
}

func TestFirestoreFieldsAndSecretRefuse(t *testing.T) {
	fields, err := ToFirestoreFields(map[string]any{
		"device":  "dev_ab",
		"updated": "2026-11-28T00:00:00Z",
		"projects": []any{
			map[string]any{"slug": "shop", "name": "Shop"},
		},
		"settings": map[string]any{"keep_deploys": 8, "rover_version": "0.9.0"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if fields["device"].(map[string]any)["stringValue"] != "dev_ab" {
		t.Fatalf("device: %v", fields["device"])
	}
	settings := fields["settings"].(map[string]any)["mapValue"].(map[string]any)["fields"].(map[string]any)
	if settings["keep_deploys"].(map[string]any)["integerValue"] != "8" {
		t.Fatalf("keep_deploys: %v", settings["keep_deploys"])
	}
	projects := fields["projects"].(map[string]any)["arrayValue"].(map[string]any)["values"].([]any)
	if len(projects) != 1 {
		t.Fatalf("projects: %v", projects)
	}
	if !ValidFirestoreID("dev_ab12") || ValidFirestoreID("a/b") || ValidFirestoreID("") || ValidFirestoreID("..") {
		t.Fatal("ValidFirestoreID")
	}
}

func TestWriteSyncDocumentMock(t *testing.T) {
	var gotPath, gotAuth, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth = r.URL.Path, r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		if r.Method != "PATCH" {
			t.Errorf("method %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"name":"projects/demo/databases/(default)/documents/rover_sync/dev_ab"}`))
	}))
	t.Cleanup(srv.Close)
	tr := &Transport{Base: map[string]string{"firestore": srv.URL}}
	payload := []byte(`{"device":"dev_ab","updated":"2026-11-28T00:00:00Z","projects":[{"slug":"shop","name":"Shop","domain":"shop.com"}],"settings":{"rover_version":"0.9.0"}}`)
	if err := WriteSyncDocument(context.Background(), tr, map[string]string{"access_token": "ya29.demo", "project_id": "demo"}, "demo", "dev_ab", payload); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/v1/projects/demo/databases/(default)/documents/rover_sync/dev_ab" {
		t.Fatalf("path %s", gotPath)
	}
	if gotAuth != "Bearer ya29.demo" {
		t.Fatalf("auth %s", gotAuth)
	}
	if !strings.Contains(gotBody, `"stringValue":"shop.com"`) || !strings.Contains(gotBody, `"stringValue":"dev_ab"`) {
		t.Fatalf("body %s", gotBody)
	}
	if strings.Contains(gotBody, "ya29") || strings.Contains(gotBody, "PRIVATE") || strings.Contains(gotBody, `"cred"`) {
		t.Fatalf("secret in firestore body: %s", gotBody)
	}
	err := WriteSyncDocument(context.Background(), tr, map[string]string{"access_token": "ya29.demo"}, "demo", "dev_ab", []byte(`{"note":"-----BEGIN PRIVATE KEY-----"}`))
	if err == nil || !strings.Contains(err.Error(), "secret") {
		t.Fatalf("wanted secret refuse, got %v", err)
	}
	if err := WriteSyncDocument(context.Background(), tr, map[string]string{"access_token": "t"}, "demo", "dev/../x", payload); err == nil {
		t.Fatal("wanted invalid id")
	}
}

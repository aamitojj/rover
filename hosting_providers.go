package main

// Hosting accounts: the catalog of providers and how each one connects.
// Rover never asks for or stores a website password and never drives a
// provider's login page. Every provider uses its official API sign-in:
//   - DigitalOcean: "Sign in with DigitalOcean" (OAuth 2 + PKCE, public
//     client registered on first use) or a personal access token.
//   - Hetzner Cloud, Vultr, Linode/Akamai, Hostinger, Scaleway, UpCloud: an
//     API token the user creates on the provider's token page.
//   - AWS Lightsail: an access key pair (IAM user with Lightsail access).
//   - Oracle Cloud: an API signing key (tenancy, user, fingerprint, key file).
//   - Contabo: the API client id/secret plus the separate *API* password.
//   - Any other VPS: IP address + SSH (password once, then Rover's key).
// Credentials are protected with protectString (DPAPI on Windows) and never
// returned to the UI or sent to any AI.

type CredField struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Secret      bool     `json:"secret,omitempty"`
	Multiline   bool     `json:"multiline,omitempty"`
	Placeholder string   `json:"placeholder,omitempty"`
	Hint        string   `json:"hint,omitempty"`
	Optional    bool     `json:"optional,omitempty"`
	Options     []string `json:"options,omitempty"`
}

type HostingProvider struct {
	ID        string      `json:"id"`
	Name      string      `json:"name"`
	Short     string      `json:"short"` // logo letters
	Color     string      `json:"color"`
	Auth      string      `json:"auth"` // oauth | token | keys | ssh
	OAuth     bool        `json:"oauth,omitempty"`
	TokenURL  string      `json:"token_url,omitempty"`
	TokenName string      `json:"token_name,omitempty"`
	Steps     []string    `json:"steps,omitempty"`
	Fields    []CredField `json:"fields,omitempty"`
	CanList   bool        `json:"can_list"`
	CanCreate bool        `json:"can_create"`
	OrderURL  string      `json:"order_url,omitempty"`
	Note      string      `json:"note,omitempty"`
	From      string      `json:"from,omitempty"` // rough starting price, shown as a hint only
	User      string      `json:"user"`           // default SSH user of its Ubuntu images
	Docs      string      `json:"docs,omitempty"`
}

var tokenField = CredField{Key: "token", Label: "API token", Secret: true, Placeholder: "paste the token here"}

var hostingProviders = []HostingProvider{
	{ID: "digitalocean", Name: "DigitalOcean", Short: "DO", Color: "#0069ff", Auth: "oauth", OAuth: true, TokenURL: "https://cloud.digitalocean.com/account/api/tokens", TokenName: "personal access token",
		Steps:  []string{"Click Sign in with DigitalOcean. Your browser opens DigitalOcean's own sign-in page.", "Sign in there (2-step login works as usual) and click Authorize.", "Come back to Rover: your account and servers appear by themselves."},
		Fields: []CredField{tokenField}, CanList: true, CanCreate: true, From: "about $6/month", User: "root", OrderURL: "https://cloud.digitalocean.com/droplets/new", Docs: "https://docs.digitalocean.com/reference/api/oauth/",
		Note: "Prefer a token? Open the token page, click Generate New Token, choose Full Access (or read + write for Droplets and SSH keys) and paste it."},
	{ID: "hetzner", Name: "Hetzner Cloud", Short: "H", Color: "#d50c2d", Auth: "token", TokenURL: "https://console.hetzner.cloud/projects", TokenName: "API token",
		Steps:  []string{"Click Open the token page and pick your project.", "Go to Security > API tokens > Generate API token.", "Choose Read & Write, give it a name like Rover, and copy it.", "Paste it below. Each Hetzner project needs its own token, so add one account per project."},
		Fields: []CredField{tokenField}, CanList: true, CanCreate: true, From: "about €4/month", User: "root", OrderURL: "https://console.hetzner.cloud/", Docs: "https://docs.hetzner.cloud/"},
	{ID: "vultr", Name: "Vultr", Short: "V", Color: "#007bfc", Auth: "token", TokenURL: "https://my.vultr.com/settings/#settingsapi", TokenName: "API key",
		Steps:  []string{"Click Open the token page (Account > API).", "Click Enable API and copy the API key.", "Under Access Control, allow your PC's internet address (or All IPv4) so Rover can use the key.", "Paste the key below."},
		Fields: []CredField{{Key: "token", Label: "API key", Secret: true}}, CanList: true, CanCreate: true, From: "about $6/month", User: "root", OrderURL: "https://my.vultr.com/deploy/", Docs: "https://www.vultr.com/api/"},
	{ID: "linode", Name: "Linode (Akamai)", Short: "L", Color: "#00a95c", Auth: "token", TokenURL: "https://cloud.linode.com/profile/tokens", TokenName: "personal access token",
		Steps:  []string{"Click Open the token page (My Profile > API Tokens).", "Click Create a Personal Access Token.", "Set Linodes to Read/Write (everything else can stay No Access) and pick an expiry.", "Copy the token (shown once) and paste it below."},
		Fields: []CredField{tokenField}, CanList: true, CanCreate: true, From: "about $5/month", User: "root", OrderURL: "https://cloud.linode.com/linodes/create", Docs: "https://techdocs.akamai.com/linode-api/reference/api"},
	{ID: "lightsail", Name: "AWS Lightsail", Short: "AWS", Color: "#ff9900", Auth: "keys", TokenURL: "https://console.aws.amazon.com/iam/home#/users", TokenName: "access key",
		Steps:   []string{"Click Open the token page (AWS IAM > Users).", "Create a user just for Rover and attach a policy that allows Lightsail (for example AmazonLightsailFullAccess, or a custom lightsail:* policy).", "Open the user > Security credentials > Create access key > Application running outside AWS.", "Copy the Access key ID and Secret access key and paste them below. Pick the region your servers are in."},
		Fields:  []CredField{{Key: "access_key", Label: "Access key ID", Placeholder: "AKIA..."}, {Key: "secret_key", Label: "Secret access key", Secret: true}, {Key: "region", Label: "Region", Options: []string{"ap-south-1", "us-east-1", "us-east-2", "us-west-2", "eu-west-1", "eu-west-2", "eu-central-1", "ap-southeast-1", "ap-northeast-1", "ap-southeast-2", "ca-central-1"}}},
		CanList: true, CanCreate: true, From: "about $7/month for 2 GB", User: "ubuntu", OrderURL: "https://lightsail.aws.amazon.com/", Docs: "https://docs.aws.amazon.com/lightsail/2016-11-28/api-reference/"},
	{ID: "oracle", Name: "Oracle Cloud (OCI)", Short: "OCI", Color: "#c74634", Auth: "keys", TokenURL: "https://cloud.oracle.com/identity/domains/my-profile/api-keys", TokenName: "API signing key",
		Steps:   []string{"Click Open the token page (Profile > API keys).", "Click Add API key > Generate API key pair > Download private key, then Add.", "Oracle shows a Configuration file preview: copy the user, fingerprint, tenancy and region values below.", "Open the downloaded .pem file in Notepad and paste all of it into Private key."},
		Fields:  []CredField{{Key: "tenancy", Label: "Tenancy OCID", Placeholder: "ocid1.tenancy.oc1..."}, {Key: "user", Label: "User OCID", Placeholder: "ocid1.user.oc1..."}, {Key: "fingerprint", Label: "Fingerprint", Placeholder: "12:34:..."}, {Key: "region", Label: "Region", Placeholder: "ap-mumbai-1"}, {Key: "key", Label: "Private key (.pem)", Secret: true, Multiline: true, Placeholder: "-----BEGIN PRIVATE KEY-----"}},
		CanList: true, CanCreate: false, From: "Always Free Ampere A1 available", User: "ubuntu", OrderURL: "https://cloud.oracle.com/compute/instances/create", Docs: "https://docs.oracle.com/en-us/iaas/Content/API/Concepts/signingrequests.htm",
		Note: "Create the server in the Oracle console (choose Ubuntu and Ampere A1 for Always Free, and paste Rover's public key shown below). Free capacity is often busy, so Rover does not create Oracle servers itself. It lists them and sets them up."},
	{ID: "contabo", Name: "Contabo", Short: "C", Color: "#1f2f5a", Auth: "keys", TokenURL: "https://new.contabo.com/account/security", TokenName: "API credentials",
		Steps:   []string{"Click Open the token page (Account > Security & Access).", "Copy the Client ID and Client Secret.", "Click Send link to set an API password. This is a separate password just for the API, not your login password.", "Paste all four values below (API user is your Contabo e-mail)."},
		Fields:  []CredField{{Key: "client_id", Label: "Client ID"}, {Key: "client_secret", Label: "Client secret", Secret: true}, {Key: "user", Label: "API user (your e-mail)"}, {Key: "password", Label: "API password (not your login password)", Secret: true}},
		CanList: true, CanCreate: false, From: "about €5/month", User: "root", OrderURL: "https://contabo.com/en/vps/", Docs: "https://api.contabo.com/",
		Note: "Contabo servers are monthly contracts, so order them on contabo.com. Rover lists them and sets them up."},
	{ID: "hostinger", Name: "Hostinger VPS", Short: "Hs", Color: "#673de6", Auth: "token", TokenURL: "https://hpanel.hostinger.com/api", TokenName: "API token",
		Steps:  []string{"Click Open the token page (hPanel > API).", "Click Generate new token, name it Rover and choose an expiry.", "Copy the token (shown once) and paste it below."},
		Fields: []CredField{tokenField}, CanList: true, CanCreate: false, From: "from about ₹400/month", User: "root", OrderURL: "https://www.hostinger.com/vps-hosting", Docs: "https://developers.hostinger.com/",
		Note: "Buying a VPS through the API charges your default payment method right away, so Rover leaves ordering to hPanel. It lists your VPS servers and sets them up."},
	{ID: "scaleway", Name: "Scaleway", Short: "S", Color: "#4f0599", Auth: "token", TokenURL: "https://console.scaleway.com/iam/api-keys", TokenName: "API key (secret key)",
		Steps:  []string{"Click Open the token page (IAM > API keys).", "Click Generate an API key, pick yourself as the bearer and copy the Secret key.", "Paste the secret key below."},
		Fields: []CredField{{Key: "token", Label: "Secret key", Secret: true}}, CanList: true, CanCreate: false, From: "about €7/month", User: "root", OrderURL: "https://console.scaleway.com/instance/servers/create", Docs: "https://www.scaleway.com/en/developers/api/instances/"},
	{ID: "upcloud", Name: "UpCloud", Short: "U", Color: "#7b00ff", Auth: "token", TokenURL: "https://hub.upcloud.com/account/api-tokens", TokenName: "API token (ucat_...)",
		Steps:  []string{"Click Open the token page (Account > API tokens).", "Create a token, allow access from all addresses (or your PC's address) and copy it. It starts with ucat_.", "Paste it below."},
		Fields: []CredField{tokenField}, CanList: true, CanCreate: false, From: "about €7/month", User: "root", OrderURL: "https://hub.upcloud.com/deploy", Docs: "https://developers.upcloud.com/api/1.3/"},
	{ID: "other", Name: "Any other VPS", Short: "SSH", Color: "#475569", Auth: "ssh",
		Steps:   []string{"Have the server's IP address and its root password (or an SSH key) ready. Ubuntu 22.04 or 24.04 works best.", "Rover logs in once with them and installs its own SSH key.", "After that the password is never needed again and is not saved."},
		CanList: false, CanCreate: false, User: "root"},
}

func hostingProviderByID(id string) (HostingProvider, bool) {
	for _, p := range hostingProviders {
		if p.ID == id {
			return p, true
		}
	}
	return HostingProvider{}, false
}

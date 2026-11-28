package prov

import (
	"context"
	"encoding/json"
	"strings"
)

func init() { Register(firebaseProv{}) }

type firebaseProv struct{}

func (firebaseProv) Meta() Meta {
	return Meta{
		ID: "firebase", Name: "Firebase", Auth: "service_account",
		Docs: "https://firebase.google.com/docs/projects/api/reference/rest",
		Capabilities: map[Capability]Support{
			CapHosting: SupportYes, CapDatabase: SupportYes, CapAuth: SupportYes, CapStorage: SupportYes,
		},
		Fields: []Field{
			{Key: "json", Label: "Service account JSON", Secret: true, Multiline: true, Hint: "Firebase / GCP → IAM → Service accounts → Keys → JSON"},
			{Key: "project_id", Label: "Firebase project ID (if not in the JSON)", Optional: true},
		},
		Scopes: []string{
			"Service account with: firebase.projects (read), firebasehosting.readonly (or admin to deploy), datastore.viewer (Firestore read), firebaseauth.viewer (or admin), storage.objectViewer.",
			"OAuth user tokens with the same scopes are also accepted in access_token.",
		},
		Notes: "Uses official REST APIs (Firebase Management, Hosting, Firestore, Identity Toolkit). Never deploys or writes without a confirm in the app.",
	}
}

const firebaseScopes = "https://www.googleapis.com/auth/cloud-platform https://www.googleapis.com/auth/firebase https://www.googleapis.com/auth/datastore https://www.googleapis.com/auth/firebase.database https://www.googleapis.com/auth/identitytoolkit https://www.googleapis.com/auth/devstorage.read_only"

func (firebaseProv) Test(ctx context.Context, tr *Transport, creds map[string]string) (*Info, error) {
	hdr, err := googleBearer(ctx, tr, creds, firebaseScopes)
	if err != nil {
		return nil, err
	}
	project := firebaseProjectID(creds)
	mgmt := tr.BaseURL("firebase", "https://firebase.googleapis.com")
	info := &Info{Health: "ok", Status: StatusConnected, Who: project}
	if project == "" {
		var list struct {
			Results []struct {
				ProjectID   string `json:"projectId"`
				DisplayName string `json:"displayName"`
				State       string `json:"state"`
			} `json:"results"`
		}
		if err := tr.DoJSON(ctx, "GET", mgmt+"/v1beta1/projects", hdr, nil, &list); err != nil {
			return nil, err
		}
		info.Usage = map[string]any{"projects": len(list.Results)}
		for _, p := range list.Results {
			info.Resources = append(info.Resources, Resource{ID: p.ProjectID, Name: firstNonEmpty(p.DisplayName, p.ProjectID), Kind: "project", Status: p.State})
			if project == "" {
				project = p.ProjectID
				info.Who = firstNonEmpty(p.DisplayName, p.ProjectID)
			}
		}
		if project == "" {
			return info, nil
		}
	}
	host := tr.BaseURL("firebase-hosting", "https://firebasehosting.googleapis.com")
	var sites struct {
		Sites []struct {
			Name       string `json:"name"`
			DefaultURL string `json:"defaultUrl"`
			Type       string `json:"type"`
		} `json:"sites"`
	}
	if err := tr.DoJSON(ctx, "GET", host+"/v1beta1/projects/"+project+"/sites", hdr, nil, &sites); err == nil {
		for _, s := range sites.Sites {
			info.Resources = append(info.Resources, Resource{ID: s.Name, Name: s.Name, Kind: "hosting-site", URL: s.DefaultURL})
		}
		info.Usage = map[string]any{"hosting_sites": len(sites.Sites)}
	}
	fs := tr.BaseURL("firestore", "https://firestore.googleapis.com")
	var cols struct {
		CollectionIds []string `json:"collectionIds"`
	}
	if err := tr.DoJSON(ctx, "GET", fs+"/v1/projects/"+project+"/databases/(default)/documents:listCollectionIds", hdr, nil, &cols); err == nil {
		for _, c := range cols.CollectionIds {
			info.Resources = append(info.Resources, Resource{ID: c, Name: c, Kind: "collection"})
		}
	} else {
		// listCollectionIds is POST officially
		_ = tr.DoJSON(ctx, "POST", fs+"/v1/projects/"+project+"/databases/(default)/documents:listCollectionIds", hdr, map[string]any{}, &cols)
		for _, c := range cols.CollectionIds {
			info.Resources = append(info.Resources, Resource{ID: c, Name: c, Kind: "collection"})
		}
	}
	return info, nil
}

func firebaseProjectID(creds map[string]string) string {
	if p := strings.TrimSpace(creds["project_id"]); p != "" {
		return p
	}
	if raw := strings.TrimSpace(creds["json"]); raw != "" {
		var sa struct {
			ProjectID string `json:"project_id"`
		}
		_ = json.Unmarshal([]byte(raw), &sa)
		return sa.ProjectID
	}
	return ""
}

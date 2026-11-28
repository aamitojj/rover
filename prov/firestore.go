package prov

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

const firestoreSyncCollection = "rover_sync"

const firestoreWriteScope = "https://www.googleapis.com/auth/datastore"

// ToFirestoreFields converts a JSON-encodable value into a Firestore REST
// "fields" map (stringValue / integerValue / booleanValue / mapValue /
// arrayValue). Used to upsert Rover metadata documents.
func ToFirestoreFields(v any) (map[string]any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var generic any
	if err := json.Unmarshal(b, &generic); err != nil {
		return nil, err
	}
	val, err := toFSValue(generic)
	if err != nil {
		return nil, err
	}
	m, _ := val["mapValue"].(map[string]any)
	if m == nil {
		return nil, fmt.Errorf("sync payload must be a JSON object")
	}
	fields, _ := m["fields"].(map[string]any)
	if fields == nil {
		fields = map[string]any{}
	}
	return fields, nil
}

func toFSValue(v any) (map[string]any, error) {
	if v == nil {
		return map[string]any{"nullValue": "NULL_VALUE"}, nil
	}
	switch x := v.(type) {
	case bool:
		return map[string]any{"booleanValue": x}, nil
	case string:
		return map[string]any{"stringValue": x}, nil
	case float64:
		if x == float64(int64(x)) && x >= -9e15 && x <= 9e15 {
			return map[string]any{"integerValue": fmt.Sprintf("%d", int64(x))}, nil
		}
		return map[string]any{"doubleValue": x}, nil
	case json.Number:
		if i, err := x.Int64(); err == nil {
			return map[string]any{"integerValue": fmt.Sprintf("%d", i)}, nil
		}
		f, err := x.Float64()
		if err != nil {
			return nil, err
		}
		return map[string]any{"doubleValue": f}, nil
	case []any:
		vals := make([]any, 0, len(x))
		for _, el := range x {
			fv, err := toFSValue(el)
			if err != nil {
				return nil, err
			}
			vals = append(vals, fv)
		}
		return map[string]any{"arrayValue": map[string]any{"values": vals}}, nil
	case map[string]any:
		fields := map[string]any{}
		for k, el := range x {
			fv, err := toFSValue(el)
			if err != nil {
				return nil, err
			}
			fields[k] = fv
		}
		return map[string]any{"mapValue": map[string]any{"fields": fields}}, nil
	default:
		return nil, fmt.Errorf("cannot store a %T value in Firestore", v)
	}
}

// ValidFirestoreID reports whether s is a safe document or collection id
// (no slashes, no path traversal).
func ValidFirestoreID(s string) bool {
	if s == "" || s == "." || s == ".." || len(s) > 1500 {
		return false
	}
	for _, r := range s {
		if r == '/' || r == '\\' || r == '?' || r == '#' || r == '\x00' {
			return false
		}
	}
	return true
}

// WriteSyncDocument upserts a metadata-only JSON payload into
// projects/{project}/databases/(default)/documents/rover_sync/{device}
// using the official Firestore REST API. The payload is checked again for
// secrets and refused if it looks like it contains a key.
func WriteSyncDocument(ctx context.Context, tr *Transport, creds map[string]string, project, device string, payloadJSON []byte) error {
	if !ValidFirestoreID(project) {
		return fmt.Errorf("the Firebase project id is not valid")
	}
	if !ValidFirestoreID(device) {
		return fmt.Errorf("the sync device id is not a valid Firestore document name")
	}
	if firestoreLooksSecret(payloadJSON) {
		return fmt.Errorf("sync refused: the payload looked like it contained a secret")
	}
	var payload any
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return fmt.Errorf("the sync payload is not valid JSON")
	}
	fields, err := ToFirestoreFields(payload)
	if err != nil {
		return err
	}
	doc := map[string]any{"fields": fields}
	body, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	if firestoreLooksSecret(body) {
		return fmt.Errorf("sync refused: the payload looked like it contained a secret")
	}
	hdr, err := googleBearer(ctx, tr, creds, firestoreWriteScope)
	if err != nil {
		return err
	}
	base := tr.BaseURL("firestore", "https://firestore.googleapis.com")
	u := base + "/v1/projects/" + url.PathEscape(project) + "/databases/(default)/documents/" +
		url.PathEscape(firestoreSyncCollection) + "/" + url.PathEscape(device)
	return tr.DoJSON(ctx, "PATCH", u, hdr, doc, nil)
}

func firestoreLooksSecret(b []byte) bool {
	s := string(b)
	return (strings.Contains(s, "BEGIN ") && strings.Contains(s, "PRIVATE KEY")) ||
		strings.Contains(s, `"cred"`) ||
		strings.Contains(s, "dpapi:") ||
		strings.Contains(s, "coolify_token") ||
		strings.Contains(s, "managed_key") ||
		strings.Contains(s, "-----BEGIN")
}

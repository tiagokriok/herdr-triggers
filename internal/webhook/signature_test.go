package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"os"
	"strings"
	"testing"
)

const secret = "whsec-0123456789abcdef"

func sign(body string) []byte {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(body))
	return m.Sum(nil)
}

func (e *env) signed(signature string) {
	hook := `{"id":"signed","title":"S","prompt":"p","cwd":"/tmp","schedule":{"type":"webhook","token":"` + token + `","signature":` + signature + `},"enabled":true}`
	os.WriteFile(e.tasks, []byte(`{"tasks":[`+hook+`]}`), 0o644)
}

func TestSignatureGitHub(t *testing.T) {
	e := newEnv(t)
	e.signed(`{"secret":"` + secret + `","header":"X-Hub-Signature-256","encoding":"hex","prefix":"sha256="}`)
	body := `{"action":"opened"}`
	good := map[string]string{"X-Hub-Signature-256": "sha256=" + hex.EncodeToString(sign(body))}
	if r := e.do("POST", "/hooks/signed/"+token, body, good); r.Code != 202 {
		t.Fatalf("valid signature: %d %s", r.Code, r.Body)
	}
	e.wg.Wait()
	cases := map[string]map[string]string{
		"missing header": nil,
		"wrong body":     {"X-Hub-Signature-256": "sha256=" + hex.EncodeToString(sign("other"))},
		"no prefix":      {"X-Hub-Signature-256": hex.EncodeToString(sign(body))},
		"garbage":        {"X-Hub-Signature-256": "sha256=zz"},
	}
	for name, headers := range cases {
		if r := e.do("POST", "/hooks/signed/"+token, body, headers); r.Code != 404 {
			t.Fatalf("%s: got %d want 404", name, r.Code)
		}
	}
	if r := e.do("POST", "/hooks/signed/nope", body, good); r.Code != 404 {
		t.Fatalf("signed but wrong token: %d", r.Code)
	}
	ds, _ := e.db.Deliveries("signed", 10)
	if ds[0].Outcome != "unauthorized" || ds[1].Outcome != "bad_signature" || ds[len(ds)-1].Outcome != "accepted" {
		t.Fatalf("%+v", ds)
	}
}

func TestSignatureLinearAndBase64(t *testing.T) {
	e := newEnv(t)
	e.signed(`{"secret":"` + secret + `","header":"Linear-Signature","encoding":"hex"}`)
	body := `{"type":"Issue"}`
	if r := e.do("POST", "/hooks/signed/"+token, body, map[string]string{"Linear-Signature": hex.EncodeToString(sign(body))}); r.Code != 202 {
		t.Fatalf("linear: %d", r.Code)
	}
	e.wg.Wait()
	e.signed(`{"secret":"` + secret + `","header":"X-Sig","encoding":"base64"}`)
	if r := e.do("POST", "/hooks/signed/"+token, body, map[string]string{"X-Sig": base64.StdEncoding.EncodeToString(sign(body))}); r.Code != 202 {
		t.Fatalf("base64: %d", r.Code)
	}
	e.wg.Wait()
}

func TestNotFoundLooksTheSame(t *testing.T) {
	e := newEnv(t)
	unknown := e.do("POST", "/hooks/nope", "", bearer)
	wrong := e.do("POST", "/hooks/alert/nope", "", nil)
	if unknown.Code != wrong.Code || strings.TrimSpace(unknown.Body.String()) != strings.TrimSpace(wrong.Body.String()) {
		t.Fatalf("unknown %d %q vs wrong token %d %q", unknown.Code, unknown.Body, wrong.Code, wrong.Body)
	}
}

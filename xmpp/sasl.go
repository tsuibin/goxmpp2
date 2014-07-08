// Deal with SASL authentication.

package xmpp

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"math/big"
	"regexp"
	"strings"
)

// Server is advertising auth mechanisms it supports. Choose one and
// respond.
// BUG(cjyar): Doesn't implement TLS/SASL EXTERNAL.
func (cl *Client) chooseSasl(fe *Features) {
	var digestMd5, plain bool
	var mechs []string
	for _, m := range fe.Mechanisms.Mechanism {
		mechs = append(mechs, m)
		switch strings.ToLower(m) {
		case "digest-md5":
			digestMd5 = true
		case "plain":
			plain = true
		}
	}

	if digestMd5 {
		auth := &auth{XMLName: xml.Name{Space: NsSASL, Local: "auth"},
			Mechanism: "DIGEST-MD5"}
		cl.sendRaw <- auth
	} else if plain {
		raw := "\x00" + cl.Jid.Node() + "\x00" + cl.password
		enc := base64.StdEncoding.EncodeToString([]byte(raw))
		auth := &auth{XMLName: xml.Name{Space: NsSASL, Local: "auth"},
			Mechanism: "PLAIN", Chardata: enc}
		cl.sendRaw <- auth
	} else {
		cl.setError(fmt.Errorf("No supported auth mechanism in %v",
			mechs))
	}
}

// Server is responding to our auth request.
func (cl *Client) handleSasl(srv *auth) {
	switch strings.ToLower(srv.XMLName.Local) {
	case "challenge":
		b64 := base64.StdEncoding
		str, err := b64.DecodeString(srv.Chardata)
		if err != nil {
			cl.setError(fmt.Errorf("SASL: %v", err))
			return
		}
		srvMap := parseSasl(string(str))

		if cl.saslExpected == "" {
			cl.saslDigest1(srvMap)
		} else {
			cl.saslDigest2(srvMap)
		}
	case "failure":
		cl.setError(fmt.Errorf("SASL authentication failed"))
	case "success":
		cl.setStatus(StatusAuthenticated)
		cl.Features = nil
		ss := &stream{To: cl.Jid.Domain(), Version: XMPPVersion}
		cl.sendRaw <- ss
	}
}

func (cl *Client) saslDigest1(srvMap map[string]string) {
	// Make sure it supports qop=auth
	var hasAuth bool
	for _, qop := range strings.Fields(srvMap["qop"]) {
		if qop == "auth" {
			hasAuth = true
		}
	}
	if !hasAuth {
		cl.setError(fmt.Errorf("Server doesn't support SASL auth"))
		return
	}

	// Pick a realm.
	var realm string
	if srvMap["realm"] != "" {
		realm = strings.Fields(srvMap["realm"])[0]
	}

	passwd := cl.password
	nonce := srvMap["nonce"]
	digestUri := "xmpp/" + cl.Jid.Domain()
	nonceCount := int32(1)
	nonceCountStr := fmt.Sprintf("%08x", nonceCount)

	// Begin building the response. Username is
	// user@domain or just domain.
	var username string
	if cl.Jid.Node() == "" {
		username = cl.Jid.Domain()
	} else {
		username = cl.Jid.Node()
	}

	// Generate our own nonce from random data.
	randSize := big.NewInt(0)
	randSize.Lsh(big.NewInt(1), 64)
	cnonce, err := rand.Int(rand.Reader, randSize)
	if err != nil {
		cl.setError(fmt.Errorf("SASL rand: %v", err))
		return
	}
	cnonceStr := fmt.Sprintf("%016x", cnonce)

	/* Now encode the actual password response, as well as the
	 * expected next challenge from the server. */
	response := saslDigestResponse(username, realm, passwd, nonce,
		cnonceStr, "AUTHENTICATE", digestUri, nonceCountStr)
	next := saslDigestResponse(username, realm, passwd, nonce,
		cnonceStr, "", digestUri, nonceCountStr)
	cl.saslExpected = next

	// Build the map which will be encoded.
	clMap := make(map[string]string)
	clMap["realm"] = `"` + realm + `"`
	clMap["username"] = `"` + username + `"`
	clMap["nonce"] = `"` + nonce + `"`
	clMap["cnonce"] = `"` + cnonceStr + `"`
	clMap["nc"] = nonceCountStr
	clMap["qop"] = "auth"
	clMap["digest-uri"] = `"` + digestUri + `"`
	clMap["response"] = response
	if srvMap["charset"] == "utf-8" {
		clMap["charset"] = "utf-8"
	}

	// Encode the map and send it.
	clStr := packSasl(clMap)
	b64 := base64.StdEncoding
	clObj := &auth{XMLName: xml.Name{Space: NsSASL, Local: "response"},
		Chardata: b64.EncodeToString([]byte(clStr))}
	cl.sendRaw <- clObj
}

func (cl *Client) saslDigest2(srvMap map[string]string) {
	if cl.saslExpected == srvMap["rspauth"] {
		clObj := &auth{XMLName: xml.Name{Space: NsSASL, Local: "response"}}
		cl.sendRaw <- clObj
	} else {
		clObj := &auth{XMLName: xml.Name{Space: NsSASL, Local: "failure"}, Any: &Generic{XMLName: xml.Name{Space: NsSASL,
			Local: "abort"}}}
		cl.sendRaw <- clObj
	}
}

// Takes a string like `key1=value1,key2="value2"...` and returns a
// key/value map.
func parseSasl(in string) map[string]string {
	re := regexp.MustCompile(`([^=]+)="?([^",]+)"?,?`)
	strs := re.FindAllStringSubmatch(in, -1)
	m := make(map[string]string)
	for _, pair := range strs {
		key := strings.ToLower(string(pair[1]))
		value := string(pair[2])
		m[key] = value
	}
	return m
}

// Inverse of parseSasl().
func packSasl(m map[string]string) string {
	var terms []string
	for key, value := range m {
		if key == "" || value == "" || value == `""` {
			continue
		}
		terms = append(terms, key+"="+value)
	}
	return strings.Join(terms, ",")
}

// Computes the response string for digest authentication.
func saslDigestResponse(username, realm, passwd, nonce, cnonceStr,
	authenticate, digestUri, nonceCountStr string) string {
	h := func(text string) string {
		h := md5.New()
		h.Write([]byte(text))
		return string(h.Sum(nil))
	}
	hex := func(input string) string {
		return fmt.Sprintf("%x", input)
	}
	kd := func(secret, data string) string {
		return h(secret + ":" + data)
	}

	a1 := h(username+":"+realm+":"+passwd) + ":" +
		nonce + ":" + cnonceStr
	a2 := authenticate + ":" + digestUri
	response := hex(kd(hex(h(a1)), nonce+":"+
		nonceCountStr+":"+cnonceStr+":auth:"+
		hex(h(a2))))
	return response
}

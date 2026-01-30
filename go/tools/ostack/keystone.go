package ostack

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

type keystoneAuthReq struct {
	Auth struct {
		Identity struct {
			Methods  []string `json:"methods"`
			Password struct {
				User struct {
					Name     string `json:"name"`
					Domain   struct{ Name string `json:"name"` } `json:"domain"`
					Password string `json:"password"`
				} `json:"user"`
			} `json:"password"`
		} `json:"identity"`
		Scope struct {
			Project struct {
				Name   string `json:"name"`
				Domain struct{ Name string `json:"name"` } `json:"domain"`
			} `json:"project"`
		} `json:"scope"`
	} `json:"auth"`
}

// GetTokenAndCatalog authenticates with Keystone and returns the token and catalog JSON body.
func GetTokenAndCatalog(client *http.Client, cfg *Config) (token string, catalog []byte, err error) {
	auth := keystoneAuthReq{}
	auth.Auth.Identity.Methods = []string{"password"}
	auth.Auth.Identity.Password.User.Name = cfg.User
	auth.Auth.Identity.Password.User.Domain.Name = cfg.Domain
	auth.Auth.Identity.Password.User.Password = cfg.Password
	auth.Auth.Scope.Project.Name = cfg.Project
	auth.Auth.Scope.Project.Domain.Name = cfg.Domain
	body, _ := json.Marshal(auth)
	url := strings.TrimSuffix(cfg.KeystoneURL, "/") + "/auth/tokens"
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	token = resp.Header.Get("X-Subject-Token")
	if token == "" {
		return "", nil, fmt.Errorf("no X-Subject-Token in response")
	}
	catalog, err = io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, err
	}
	if resp.StatusCode >= 400 {
		return "", nil, fmt.Errorf("keystone auth: HTTP %d", resp.StatusCode)
	}
	return token, catalog, nil
}

type catalogEntry struct {
	Type      string `json:"type"`
	Name      string `json:"name"`
	Endpoints []struct {
		Interface string `json:"interface"`
		URL       string `json:"url"`
	} `json:"endpoints"`
}

func discoverEndpoint(catalogJSON []byte, serviceName string) string {
	var top struct {
		Token *struct {
			Catalog        []catalogEntry `json:"catalog"`
			ServiceCatalog []catalogEntry `json:"service_catalog"`
		} `json:"token"`
		Catalog         []catalogEntry `json:"catalog"`
		ServiceCatalog  []catalogEntry `json:"service_catalog"`
	}
	if err := json.Unmarshal(catalogJSON, &top); err != nil {
		return ""
	}
	var entries []catalogEntry
	if top.Token != nil {
		entries = append(entries, top.Token.Catalog...)
		entries = append(entries, top.Token.ServiceCatalog...)
	}
	entries = append(entries, top.Catalog...)
	entries = append(entries, top.ServiceCatalog...)
	serviceLower := strings.ToLower(serviceName)
	for _, e := range entries {
		t := strings.ToLower(e.Type)
		n := strings.ToLower(e.Name)
		if !strings.Contains(t, serviceLower) && n != serviceLower {
			continue
		}
		for _, ep := range e.Endpoints {
			if strings.ToLower(ep.Interface) == "public" && ep.URL != "" {
				return strings.TrimSuffix(ep.URL, "/")
			}
		}
		for _, ep := range e.Endpoints {
			if ep.URL != "" {
				return strings.TrimSuffix(ep.URL, "/")
			}
		}
	}
	return ""
}

// DiscoverServiceEndpoints fills cfg.CinderURL, cfg.NovaURL, cfg.GlanceURL from the catalog when empty.
// Returns an error if a required endpoint cannot be discovered.
func DiscoverServiceEndpoints(cfg *Config, catalog []byte) error {
	log.Println("Discovering endpoints from Keystone catalog...")
	if cfg.CinderURL == "" {
		base := discoverEndpoint(catalog, "cinder")
		if base == "" {
			base = discoverEndpoint(catalog, "volumev3")
		}
		if base != "" {
			cfg.CinderURL = base + "/" + cfg.Project
			log.Printf("  ✓ Cinder: %s", cfg.CinderURL)
		} else {
			return fmt.Errorf("failed to discover Cinder")
		}
	}
	if cfg.NovaURL == "" {
		base := discoverEndpoint(catalog, "compute")
		if base == "" {
			base = discoverEndpoint(catalog, "nova")
		}
		if base != "" {
			if strings.HasSuffix(base, "/v2.1") || strings.Contains(base, "/v2.1/") {
				cfg.NovaURL = base + "/" + cfg.Project
			} else {
				cfg.NovaURL = base + "/v2.1/" + cfg.Project
			}
			log.Printf("  ✓ Nova: %s", cfg.NovaURL)
		} else {
			return fmt.Errorf("failed to discover Nova")
		}
	}
	if cfg.GlanceURL == "" {
		base := discoverEndpoint(catalog, "image")
		if base == "" {
			base = discoverEndpoint(catalog, "glance")
		}
		if base != "" {
			if strings.HasSuffix(base, "/v2") {
				cfg.GlanceURL = base + "/images"
			} else {
				cfg.GlanceURL = base + "/v2/images"
			}
			log.Printf("  ✓ Glance: %s", cfg.GlanceURL)
		} else {
			return fmt.Errorf("failed to discover Glance")
		}
	}
	return nil
}

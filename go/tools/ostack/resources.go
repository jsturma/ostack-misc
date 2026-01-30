package ostack

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// WaitStatus polls a Cinder or Glance resource until it reaches wantStatus or times out.
// timeout and interval are taken from config (e.g. time.Duration(cfg.StatusTimeoutSec)*time.Second).
func WaitStatus(client *http.Client, typ, id, wantStatus, cinderURL, glanceURL, token string, timeout, interval time.Duration) error {
	var url string
	switch typ {
	case "volume", "snapshot":
		url = cinderURL + "/" + typ + "s/" + id
	case "image":
		url = glanceURL + "/" + id
	default:
		return fmt.Errorf("unknown type: %s", typ)
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		data, err := apiGet(client, url, token)
		if err != nil {
			return err
		}
		var m map[string]interface{}
		if err := json.Unmarshal(data, &m); err != nil {
			return err
		}
		obj, _ := m[typ].(map[string]interface{})
		if obj == nil {
			obj = m
		}
		current, _ := obj["status"].(string)
		if current == "" {
			return fmt.Errorf("no status for %s %s", typ, id)
		}
		if current == wantStatus {
			log.Printf("%s %s is %s", typ, id, current)
			return nil
		}
		if current == "error" {
			return fmt.Errorf("%s %s entered error state", typ, id)
		}
		elapsed := timeout - time.Until(deadline)
		if int(elapsed.Seconds())%30 == 0 && elapsed > 0 {
			log.Printf("Waiting... (%ds, status: %s)", int(elapsed.Seconds()), current)
		}
		time.Sleep(interval)
	}
	return fmt.Errorf("timeout waiting for %s %s", typ, id)
}

// CleanupResource deletes a Cinder or Glance resource (image, volume, snapshot).
func CleanupResource(client *http.Client, typ, id, cinderURL, glanceURL, token string) {
	var url string
	switch typ {
	case "image":
		url = glanceURL + "/" + id
	case "volume":
		url = cinderURL + "/volumes/" + id
	case "snapshot":
		url = cinderURL + "/snapshots/" + id
	default:
		return
	}
	log.Printf("Cleaning up %s: %s", typ, id)
	req, _ := http.NewRequest(http.MethodDelete, url, nil)
	req.Header.Set("X-Auth-Token", token)
	resp, err := client.Do(req)
	if err != nil || resp != nil && resp.StatusCode >= 400 {
		log.Printf("Warning: Failed to delete %s: %s", typ, id)
	}
	if resp != nil {
		resp.Body.Close()
	}
}

package main

import (
	"fmt"
	"net"
	"crypto/tls"
	"encoding/json"

	"github.com/stormbit/ircd-healthcheck"
	"github.com/apex/go-apex"
)

func runJob(event json.RawMessage, ctx *apex.Context) (interface{}, error) {
	var err error

	// get config
	config := make(map[string]interface{})
	if err = json.Unmarshal(event, &config); err != nil {
		return nil, err
	}

	// check params
	if _, ok := config["server"]; !ok {
		return nil, fmt.Errorf("'server' not provided")
	}

	if _, ok := config["secure"]; !ok {
		config["secure"] = false
	}

	if _, ok := config["skip-verification"]; !ok {
		config["skip-verification"] = false
	}

	// connect
	var conn net.Conn
	if config["secure"].(bool) {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: config["skip-verification"].(bool),
		}

		conn, err = tls.Dial("tcp", config["server"].(string), tlsConfig)
	} else {
		conn, err = net.Dial("tcp", config["server"].(string))
	}

	if err != nil {
		return nil, err
	}

	// run check
	failure, err := healthcheck.RunHealthcheck(conn)
	if err != nil {
		return nil, err
	}

	if failure {
		return nil, fmt.Errorf("Failed to connect to server")
	}

	return "Connection successful", nil
}

func main() {
	apex.HandleFunc(runJob)
}

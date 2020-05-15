package osbapi

import (
	osb "github.com/kubernetes-sigs/go-open-service-broker-client/v2"
)

func GetClient(url, username, password string) (osb.Client, error) {
	config := osb.DefaultClientConfiguration()
	config.AuthConfig = &osb.AuthConfig{
		BasicAuthConfig: &osb.BasicAuthConfig{
			Username: username,
			Password: password,
		},
	}
	config.URL = url

	client, err := osb.NewClient(config)
	if err != nil {
		return nil, err
	}
	return client, nil
}

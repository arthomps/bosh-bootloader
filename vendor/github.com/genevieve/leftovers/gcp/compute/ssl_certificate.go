package compute

import "fmt"

type SslCertificate struct {
	client sslCertificatesClient
	name   string
	kind   string
}

func NewSslCertificate(client sslCertificatesClient, name string) SslCertificate {
	return SslCertificate{
		client: client,
		name:   name,
		kind:   "compute-ssl-certificate",
	}
}

func (s SslCertificate) Delete() error {
	err := s.client.DeleteSslCertificate(s.name)
	if err != nil {
		return fmt.Errorf("Delete: %s", err)
	}

	return nil
}

func (s SslCertificate) Name() string {
	return s.name
}

func (s SslCertificate) Type() string {
	return "Compute Ssl Certificate"
}

func (s SslCertificate) Kind() string {
	return s.kind
}

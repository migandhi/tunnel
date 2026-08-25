package store

import (
	"fmt"
	"regexp"
)

var subdomainRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

var reservedSubdomains = map[string]bool{
	"www": true, "admin": true, "api": true, "mail": true, "smtp": true,
	"imap": true, "pop": true, "mx": true, "ns": true, "ns1": true, "ns2": true,
	"ftp": true, "autoconfig": true, "autodiscover": true, "webmail": true,
	"status": true, "dashboard": true, "download": true, "downloads": true,
	"cdn": true, "assets": true, "static": true, "test": true, "dev": true,
	"staging": true, "internal": true, "localhost": true, "root": true,
}

func ValidateSubdomain(s string) error {
	if len(s) < 1 || len(s) > 63 {
		return fmt.Errorf("subdomain must be 1-63 characters")
	}
	if !subdomainRe.MatchString(s) {
		return fmt.Errorf("subdomain may contain only lowercase letters, digits and hyphens")
	}
	if reservedSubdomains[s] {
		return fmt.Errorf("subdomain %q is reserved", s)
	}
	return nil
}

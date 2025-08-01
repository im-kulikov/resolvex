package domain

import (
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"

	"github.com/im-kulikov/go-bones"
	"golang.org/x/net/idna"
)

const ErrInvalidDomain bones.Error = "invalid domain name"

var domainRegexp = regexp.MustCompile(
	`^([a-zA-Z0-9_]{1}[a-zA-Z0-9_-]{0,62}){1}(\.[a-zA-Z0-9_]{1}[a-zA-Z0-9_-]{0,62})*[._]?$`)

// Validate will validate the given string as a DNS name.
func Validate(domain string) error {
	if domain == "" || len(strings.ReplaceAll(domain, ".", "")) > 255 {
		return fmt.Errorf("%w: domain is empty", ErrInvalidDomain)
	}

	if value, err := idna.Lookup.ToASCII(domain); err != nil {
		return errors.Join(ErrInvalidDomain, err)
	} else if domainRegexp.MatchString(value) {
		return nil
	} else if net.ParseIP(domain) != nil {
		return nil
	}

	return ErrInvalidDomain
}

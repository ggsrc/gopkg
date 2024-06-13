package goodns

import (
	"context"
	"errors"
	"net"
	"strconv"

	"github.com/miekg/dns"
)

// LookupA query with specific dns server
func LookupA(ctx context.Context, domain string, useTCP bool) ([]net.IP, error) {
	return queryA(ctx, domain, nil, useTCP)
}

// LookupAWithServer query with specific dns server
func LookupAWithServer(ctx context.Context, domain string, dnsServer string, useTCP bool) ([]net.IP, error) {
	return queryA(ctx, domain, &dnsServer, useTCP)
}

// QueryA - query A record of @p domain, from @p server, using default port :53
// No fallback, no retry.
func queryA(ctx context.Context, domain string, dnsServer *string, useTCP bool) ([]net.IP, error) {
	// create dns client
	port := 53
	server := ""
	if dnsServer == nil {
		conf, err := dns.ClientConfigFromFile("/etc/resolv.conf")
		if err != nil {
			return nil, errors.New("failed to parse /etx/resolv.conf, while server is nil")
		}
		server = conf.Servers[0]
		if i := net.ParseIP(server); i != nil {
			server = net.JoinHostPort(server, strconv.Itoa(port))
		} else {
			server = dns.Fqdn(server) + ":" + strconv.Itoa(port)
		}
	} else {
		server = net.JoinHostPort(*dnsServer, strconv.Itoa(port))
	}

	c := new(dns.Client)
	if useTCP {
		c.Net = "tcp"
	}

	// make request
	req := new(dns.Msg)
	req.Id = dns.Id()
	req.RecursionDesired = true
	req.Question = make([]dns.Question, 1)
	req.Question[0] = dns.Question{Name: dns.Fqdn(domain), Qtype: dns.TypeA, Qclass: dns.ClassINET}
	resp, _, err := c.ExchangeContext(ctx, req, server)
	if err != nil {
		return nil, err
	}

	if resp == nil {
		return nil, errors.New("unexpected empty dns response")
	}

	// make result
	ips := make([]net.IP, 0)
	for _, msg := range resp.Answer {
		if a, ok := msg.(*dns.A); ok {
			ips = append(ips, a.A)
		}
	}
	return ips, nil
}

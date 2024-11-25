// SPDX-License-Identifier: GPL-3.0-or-later

package whoisquery

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/araddon/dateparse"
	"github.com/likexian/whois"
	whoisparser "github.com/likexian/whois-parser"
)

type provider interface {
	remainingTime() (float64, error)
}

type whoisClient struct {
	domainAddress string
	client        *whois.Client
}

func newProvider(config Config) (provider, error) {
	domain := config.Source
	client := whois.NewClient()
	client.SetTimeout(config.Timeout.Duration())

	return &whoisClient{
		domainAddress: domain,
		client:        client,
	}, nil
}

func (c *whoisClient) remainingTime() (float64, error) {
	info, err := c.queryWhoisInfo()
	if err != nil {
		return 0, err
	}

	if info.Domain.ExpirationDate == "" {
		if !strings.HasPrefix(c.domainAddress, "=") {
			// some servers support requesting extended data
			// https://github.com/netdata/netdata/issues/17907#issuecomment-2171758380
			c.domainAddress = fmt.Sprintf("= %s", c.domainAddress)
			return c.remainingTime()
		}
	}

	return parseWhoisInfoExpirationDate(info)
}

func (c *whoisClient) queryWhoisInfo() (*whoisparser.WhoisInfo, error) {
	resp, err := c.client.Whois(c.domainAddress)
	if err != nil {
		return nil, err
	}

	info, err := whoisparser.Parse(resp)
	if err != nil {
		return nil, err
	}

	return &info, nil
}

func parseWhoisInfoExpirationDate(info *whoisparser.WhoisInfo) (float64, error) {
	if info == nil || info.Domain == nil {
		return 0, errors.New("nil Whois Info")
	}

	if info.Domain.ExpirationDateInTime != nil {
		return time.Until(*info.Domain.ExpirationDateInTime).Seconds(), nil
	}

	date := info.Domain.ExpirationDate
	if date == "" {
		return 0, errors.New("no expiration date")
	}

	if strings.Contains(date, " ") {
		// https://community.netdata.cloud/t/whois-query-monitor-cannot-parse-expiration-time/3485
		if v, err := time.Parse("2006.01.02 15:04:05", date); err == nil {
			return time.Until(v).Seconds(), nil
		}
	}

	expire, err := dateparse.ParseAny(date)
	if err != nil {
		return 0, err
	}

	return time.Until(expire).Seconds(), nil
}

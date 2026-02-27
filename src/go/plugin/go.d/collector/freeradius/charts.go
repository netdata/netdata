// SPDX-License-Identifier: GPL-3.0-or-later

package freeradius

import "github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"

type (
	// Charts is an alias for collectorapi.Charts
	Charts = collectorapi.Charts
	// Dims is an alias for collectorapi.Dims
	Dims = collectorapi.Dims
)

var charts = Charts{
	{
		ID:    "authentication",
		Title: "Authentication",
		Units: "packets/s",
		Fam:   "authentication",
		Ctx:   "freeradius.authentication",
		Dims: Dims{
			{ID: "access-requests", Name: "requests", Algo: collectorapi.Incremental},
			{ID: "auth-responses", Name: "responses", Algo: collectorapi.Incremental},
		},
	},
	{
		ID:    "authentication_responses",
		Title: "Authentication Responses",
		Units: "packets/s",
		Fam:   "authentication",
		Ctx:   "freeradius.authentication_access_responses",
		Dims: Dims{
			{ID: "access-accepts", Name: "accepts", Algo: collectorapi.Incremental},
			{ID: "access-rejects", Name: "rejects", Algo: collectorapi.Incremental},
			{ID: "access-challenges", Name: "challenges", Algo: collectorapi.Incremental},
		},
	},
	{
		ID:    "bad_authentication_requests",
		Title: "Bad Authentication Requests",
		Units: "packets/s",
		Fam:   "authentication",
		Ctx:   "freeradius.bad_authentication",
		Dims: Dims{
			{ID: "auth-dropped-requests", Name: "dropped", Algo: collectorapi.Incremental},
			{ID: "auth-duplicate-requests", Name: "duplicate", Algo: collectorapi.Incremental},
			{ID: "auth-invalid-requests", Name: "invalid", Algo: collectorapi.Incremental},
			{ID: "auth-malformed-requests", Name: "malformed", Algo: collectorapi.Incremental},
			{ID: "auth-unknown-types", Name: "unknown-types", Algo: collectorapi.Incremental},
		},
	},
	{
		ID:    "proxy_authentication",
		Title: "Authentication",
		Units: "packets/s",
		Fam:   "proxy authentication",
		Ctx:   "freeradius.proxy_authentication",
		Dims: Dims{
			{ID: "proxy-access-requests", Name: "requests", Algo: collectorapi.Incremental},
			{ID: "proxy-auth-responses", Name: "responses", Algo: collectorapi.Incremental},
		},
	},
	{
		ID:    "proxy_authentication_responses",
		Title: "Authentication Responses",
		Units: "packets/s",
		Fam:   "proxy authentication",
		Ctx:   "freeradius.proxy_authentication_access_responses",
		Dims: Dims{
			{ID: "proxy-access-accepts", Name: "accepts", Algo: collectorapi.Incremental},
			{ID: "proxy-access-rejects", Name: "rejects", Algo: collectorapi.Incremental},
			{ID: "proxy-access-challenges", Name: "challenges", Algo: collectorapi.Incremental},
		},
	},
	{
		ID:    "bad_proxy_authentication_requests",
		Title: "Bad Authentication Requests",
		Units: "packets/s",
		Fam:   "proxy authentication",
		Ctx:   "freeradius.proxy_bad_authentication",
		Dims: Dims{
			{ID: "proxy-auth-dropped-requests", Name: "dropped", Algo: collectorapi.Incremental},
			{ID: "proxy-auth-duplicate-requests", Name: "duplicate", Algo: collectorapi.Incremental},
			{ID: "proxy-auth-invalid-requests", Name: "invalid", Algo: collectorapi.Incremental},
			{ID: "proxy-auth-malformed-requests", Name: "malformed", Algo: collectorapi.Incremental},
			{ID: "proxy-auth-unknown-types", Name: "unknown-types", Algo: collectorapi.Incremental},
		},
	},
	{
		ID:    "accounting",
		Title: "Accounting",
		Units: "packets/s",
		Fam:   "accounting",
		Ctx:   "freeradius.accounting",
		Dims: Dims{
			{ID: "accounting-requests", Name: "requests", Algo: collectorapi.Incremental},
			{ID: "accounting-responses", Name: "responses", Algo: collectorapi.Incremental},
		},
	},
	{
		ID:    "bad_accounting_requests",
		Title: "Bad Accounting Requests",
		Units: "packets/s",
		Fam:   "accounting",
		Ctx:   "freeradius.bad_accounting",
		Dims: Dims{
			{ID: "acct-dropped-requests", Name: "dropped", Algo: collectorapi.Incremental},
			{ID: "acct-duplicate-requests", Name: "duplicate", Algo: collectorapi.Incremental},
			{ID: "acct-invalid-requests", Name: "invalid", Algo: collectorapi.Incremental},
			{ID: "acct-malformed-requests", Name: "malformed", Algo: collectorapi.Incremental},
			{ID: "acct-unknown-types", Name: "unknown-types", Algo: collectorapi.Incremental},
		},
	},
	{
		ID:    "proxy_accounting",
		Title: "Accounting",
		Units: "packets/s",
		Fam:   "proxy accounting",
		Ctx:   "freeradius.proxy_accounting",
		Dims: Dims{
			{ID: "proxy-accounting-requests", Name: "requests", Algo: collectorapi.Incremental},
			{ID: "proxy-accounting-responses", Name: "responses", Algo: collectorapi.Incremental},
		},
	},
	{
		ID:    "bad_proxy_accounting_requests",
		Title: "Bad Accounting Requests",
		Units: "packets/s",
		Fam:   "proxy accounting",
		Ctx:   "freeradius.proxy_bad_accounting",
		Dims: Dims{
			{ID: "proxy-acct-dropped-requests", Name: "dropped", Algo: collectorapi.Incremental},
			{ID: "proxy-acct-duplicate-requests", Name: "duplicate", Algo: collectorapi.Incremental},
			{ID: "proxy-acct-invalid-requests", Name: "invalid", Algo: collectorapi.Incremental},
			{ID: "proxy-acct-malformed-requests", Name: "malformed", Algo: collectorapi.Incremental},
			{ID: "proxy-acct-unknown-types", Name: "unknown-types", Algo: collectorapi.Incremental},
		},
	},
}

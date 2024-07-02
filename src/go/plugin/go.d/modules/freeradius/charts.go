// SPDX-License-Identifier: GPL-3.0-or-later

package freeradius

import "github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

type (
	// Charts is an alias for module.Charts
	Charts = module.Charts
	// Dims is an alias for module.Dims
	Dims = module.Dims
)

var charts = Charts{
	{
		ID:    "authentication",
		Title: "Authentication",
		Units: "packets/s",
		Fam:   "authentication",
		Ctx:   "freeradius.authentication",
		Dims: Dims{
			{ID: "access-requests", Name: "requests", Algo: module.Incremental},
			{ID: "auth-responses", Name: "responses", Algo: module.Incremental},
		},
	},
	{
		ID:    "authentication_responses",
		Title: "Authentication Responses",
		Units: "packets/s",
		Fam:   "authentication",
		Ctx:   "freeradius.authentication_access_responses",
		Dims: Dims{
			{ID: "access-accepts", Name: "accepts", Algo: module.Incremental},
			{ID: "access-rejects", Name: "rejects", Algo: module.Incremental},
			{ID: "access-challenges", Name: "challenges", Algo: module.Incremental},
		},
	},
	{
		ID:    "bad_authentication_requests",
		Title: "Bad Authentication Requests",
		Units: "packets/s",
		Fam:   "authentication",
		Ctx:   "freeradius.bad_authentication",
		Dims: Dims{
			{ID: "auth-dropped-requests", Name: "dropped", Algo: module.Incremental},
			{ID: "auth-duplicate-requests", Name: "duplicate", Algo: module.Incremental},
			{ID: "auth-invalid-requests", Name: "invalid", Algo: module.Incremental},
			{ID: "auth-malformed-requests", Name: "malformed", Algo: module.Incremental},
			{ID: "auth-unknown-types", Name: "unknown-types", Algo: module.Incremental},
		},
	},
	{
		ID:    "proxy_authentication",
		Title: "Authentication",
		Units: "packets/s",
		Fam:   "proxy authentication",
		Ctx:   "freeradius.proxy_authentication",
		Dims: Dims{
			{ID: "proxy-access-requests", Name: "requests", Algo: module.Incremental},
			{ID: "proxy-auth-responses", Name: "responses", Algo: module.Incremental},
		},
	},
	{
		ID:    "proxy_authentication_responses",
		Title: "Authentication Responses",
		Units: "packets/s",
		Fam:   "proxy authentication",
		Ctx:   "freeradius.proxy_authentication_access_responses",
		Dims: Dims{
			{ID: "proxy-access-accepts", Name: "accepts", Algo: module.Incremental},
			{ID: "proxy-access-rejects", Name: "rejects", Algo: module.Incremental},
			{ID: "proxy-access-challenges", Name: "challenges", Algo: module.Incremental},
		},
	},
	{
		ID:    "bad_proxy_authentication_requests",
		Title: "Bad Authentication Requests",
		Units: "packets/s",
		Fam:   "proxy authentication",
		Ctx:   "freeradius.proxy_bad_authentication",
		Dims: Dims{
			{ID: "proxy-auth-dropped-requests", Name: "dropped", Algo: module.Incremental},
			{ID: "proxy-auth-duplicate-requests", Name: "duplicate", Algo: module.Incremental},
			{ID: "proxy-auth-invalid-requests", Name: "invalid", Algo: module.Incremental},
			{ID: "proxy-auth-malformed-requests", Name: "malformed", Algo: module.Incremental},
			{ID: "proxy-auth-unknown-types", Name: "unknown-types", Algo: module.Incremental},
		},
	},
	{
		ID:    "accounting",
		Title: "Accounting",
		Units: "packets/s",
		Fam:   "accounting",
		Ctx:   "freeradius.accounting",
		Dims: Dims{
			{ID: "accounting-requests", Name: "requests", Algo: module.Incremental},
			{ID: "accounting-responses", Name: "responses", Algo: module.Incremental},
		},
	},
	{
		ID:    "bad_accounting_requests",
		Title: "Bad Accounting Requests",
		Units: "packets/s",
		Fam:   "accounting",
		Ctx:   "freeradius.bad_accounting",
		Dims: Dims{
			{ID: "acct-dropped-requests", Name: "dropped", Algo: module.Incremental},
			{ID: "acct-duplicate-requests", Name: "duplicate", Algo: module.Incremental},
			{ID: "acct-invalid-requests", Name: "invalid", Algo: module.Incremental},
			{ID: "acct-malformed-requests", Name: "malformed", Algo: module.Incremental},
			{ID: "acct-unknown-types", Name: "unknown-types", Algo: module.Incremental},
		},
	},
	{
		ID:    "proxy_accounting",
		Title: "Accounting",
		Units: "packets/s",
		Fam:   "proxy accounting",
		Ctx:   "freeradius.proxy_accounting",
		Dims: Dims{
			{ID: "proxy-accounting-requests", Name: "requests", Algo: module.Incremental},
			{ID: "proxy-accounting-responses", Name: "responses", Algo: module.Incremental},
		},
	},
	{
		ID:    "bad_proxy_accounting_requests",
		Title: "Bad Accounting Requests",
		Units: "packets/s",
		Fam:   "proxy accounting",
		Ctx:   "freeradius.proxy_bad_accounting",
		Dims: Dims{
			{ID: "proxy-acct-dropped-requests", Name: "dropped", Algo: module.Incremental},
			{ID: "proxy-acct-duplicate-requests", Name: "duplicate", Algo: module.Incremental},
			{ID: "proxy-acct-invalid-requests", Name: "invalid", Algo: module.Incremental},
			{ID: "proxy-acct-malformed-requests", Name: "malformed", Algo: module.Incremental},
			{ID: "proxy-acct-unknown-types", Name: "unknown-types", Algo: module.Incremental},
		},
	},
}

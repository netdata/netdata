// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import "strings"

var deviceCategoryToActorType = map[string]string{
	"router":                  "router",
	"gateway":                 "router",
	"layer 3 switch":          "router",
	"voip gateway":            "router",
	"switch":                  "switch",
	"bridge":                  "switch",
	"hub":                     "switch",
	"sanswitch":               "switch",
	"sanbridge":               "switch",
	"bridge/extender":         "switch",
	"firewall":                "firewall",
	"security":                "firewall",
	"access point":            "access_point",
	"wireless":                "access_point",
	"wireless lan controller": "access_point",
	"extender":                "access_point",
	"radio":                   "access_point",
	"server":                  "server",
	"file server":             "server",
	"application":             "server",
	"desktop":                 "server",
	"blade system":            "server",
	"storage":                 "storage",
	"nas":                     "storage",
	"self-contained nas":      "storage",
	"nas head":                "storage",
	"tape library":            "storage",
	"load balancer":           "load_balancer",
	"wan accelerator":         "load_balancer",
	"web caching":             "load_balancer",
	"proxy server":            "load_balancer",
	"content":                 "load_balancer",
	"printer":                 "printer",
	"ip phone":                "phone",
	"voip":                    "phone",
	"gsm":                     "phone",
	"mobile":                  "phone",
	"ups":                     "ups",
	"pdu":                     "ups",
	"power":                   "ups",
	"video":                   "camera",
	"media":                   "camera",
	"media exchange":          "camera",
	"sensor":                  "camera",
	"other":                   "device",
	"network device":          "device",
	"management":              "server",
	"management controller":   "server",
	"dslam":                   "switch",
	"access server":           "server",
	"pon":                     "switch",
	"console":                 "server",
	"module":                  "device",
	"plc":                     "device",
	"sre module":              "server",
	"chassis manager":         "server",
	"snmp managed device":     "device",
}

var deviceActorTypes = func() map[string]struct{} {
	s := map[string]struct{}{"device": {}}
	for _, v := range deviceCategoryToActorType {
		s[v] = struct{}{}
	}
	return s
}()

func resolveDeviceActorType(labels map[string]string) string {
	cat := strings.TrimSpace(labels["type"])
	if cat == "" {
		return "device"
	}
	if at, ok := deviceCategoryToActorType[strings.ToLower(cat)]; ok {
		return at
	}
	return "device"
}

func IsDeviceActorType(actorType string) bool {
	_, ok := deviceActorTypes[strings.ToLower(strings.TrimSpace(actorType))]
	return ok
}

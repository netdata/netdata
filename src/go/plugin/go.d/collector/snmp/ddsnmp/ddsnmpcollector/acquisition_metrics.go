// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

func (c *acquisitionProfileCollection) metricScalarObserver() *acquisitionScalarObserver {
	if c == nil {
		return nil
	}
	return &acquisitionScalarObserver{collection: c, routeIndexes: c.metricScalarRoutes}
}

func (c *acquisitionProfileCollection) metricTableScope() *acquisitionTableScope {
	if c == nil {
		return nil
	}
	return &acquisitionTableScope{collection: c, routeIndexes: c.metricTableRoutes}
}

func (c *acquisitionProfileCollection) addMetricValueReference(ordinal uint32, reference AcquisitionValueReference) {
	if c == nil {
		return
	}
	switch c.routes[ordinal].Kind {
	case AcquisitionRouteKindTopologyScalar, AcquisitionRouteKindTopologyTable:
		c.addTopologyValueReference(reference)
	default:
		c.addValueReference(&c.metricValueReferences, reference)
	}
}

func (c *acquisitionProfileCollection) licenseRoute(index int) *AcquisitionRouteReport {
	if c == nil {
		return nil
	}
	return c.route(c.licenseRoutes[index])
}

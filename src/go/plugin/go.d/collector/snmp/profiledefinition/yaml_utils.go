// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package profiledefinition

// StringArray is list of string with a yaml un-marshaller that support both array and string.
// See test file for example usage.
// Credit: https://github.com/go-yaml/yaml/issues/100#issuecomment-324964723
type StringArray []string

// UnmarshalYAML unmarshalls StringArray
func (a *StringArray) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var multi []string
	err := unmarshal(&multi)
	if err != nil {
		var single string
		err := unmarshal(&single)
		if err != nil {
			return err
		}
		*a = []string{single}
	} else {
		*a = multi
	}
	return nil
}

// UnmarshalYAML unmarshalls SymbolConfig
func (a *SymbolConfigCompat) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var symbol SymbolConfig
	err := unmarshal(&symbol)
	if err != nil {
		var str string
		err := unmarshal(&str)
		if err != nil {
			return err
		}
		*a = SymbolConfigCompat(SymbolConfig{Name: str})
	} else {
		*a = SymbolConfigCompat(symbol)
	}
	return nil
}

// UnmarshalYAML unmarshalls MetricTagConfigList
func (mtcl *MetricTagConfigList) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var multi []MetricTagConfig
	err := unmarshal(&multi)
	if err != nil {
		var tags []string
		err := unmarshal(&tags)
		if err != nil {
			return err
		}
		multi = []MetricTagConfig{}
		for _, tag := range tags {
			multi = append(multi, MetricTagConfig{SymbolTag: tag})
		}
	}
	*mtcl = multi
	return nil
}

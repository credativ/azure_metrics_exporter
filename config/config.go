package config

import (
	"fmt"
	"io/ioutil"
	"regexp"
	"strings"
	"sync"

	yaml "gopkg.in/yaml.v2"
)

// Config - Azure exporter configuration
type Config struct {
	Credentials    Credentials     `yaml:"credentials"`
	Resources      []Resource      `yaml:"resources"`
	ResourceGroups []ResourceGroup `yaml:"resource_groups"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
}

// SafeConfig - mutex protected config for live reloads.
type SafeConfig struct {
	sync.RWMutex
	C *Config
}

// ReloadConfig - allows for live reloads of the configuration file.
func (sc *SafeConfig) ReloadConfig(confFile string) (err error) {
	var c = &Config{}

	yamlFile, err := ioutil.ReadFile(confFile)
	if err != nil {
		return fmt.Errorf("Error reading config file: %s", err)
	}

	if err := yaml.Unmarshal(yamlFile, c); err != nil {
		return fmt.Errorf("Error parsing config file: %s", err)
	}

	if err := c.Validate(); err != nil {
		return fmt.Errorf("Error validating config file: %s", err)
	}

	sc.Lock()
	sc.C = c
	sc.Unlock()

	return nil
}

var validAggregations = []string{"Total", "Average", "Minimum", "Maximum"}

func (c *Config) validateAggregations(aggregations []string) error {
	for _, a := range aggregations {
		ok := false
		for _, valid := range validAggregations {
			if a == valid {
				ok = true
				break
			}
		}
		if !ok {
			return fmt.Errorf("%s is not one of the valid aggregations (%v)", a, validAggregations)
		}
	}

	return nil
}

func (c *Config) Validate() (err error) {
	for _, t := range c.Resources {
		if err := c.validateAggregations(t.Aggregations); err != nil {
			return err
		}

		if len(t.Name) == 0 {
			return fmt.Errorf("name needs to be specified in each resource")
		}

		if !strings.HasPrefix(t.Name, "/") {
			return fmt.Errorf("Resource path %q must start with a /", t.Name)
		}

		if len(t.Metrics) == 0 {
			return fmt.Errorf("At least one metric needs to be specified in each resource")
		}
	}

	for _, t := range c.ResourceGroups {
		if err := c.validateAggregations(t.Aggregations); err != nil {
			return err
		}

		if len(t.Name) == 0 {
			return fmt.Errorf("name needs to be specified in each resource group")
		}

		if len(t.Name) == 0 {
			return fmt.Errorf("At lease one resource type needs to be specified in each resource group")
		}

		if len(t.Metrics) == 0 {
			return fmt.Errorf("At least one metric needs to be specified in each resource group")
		}

		for _, rx := range append(t.ResourceInclude, t.ResourceExclude...) {
			if _, err := regexp.Compile(rx); err != nil {
				return fmt.Errorf("Error in regexp '%s': %s", rx, err)
			}
		}
	}

	return nil
}

// Credentials - Azure credentials
type Credentials struct {
	SubscriptionID string `yaml:"subscription_id"`
	ClientID       string `yaml:"client_id"`
	ClientSecret   string `yaml:"client_secret"`
	TenantID       string `yaml:"tenant_id"`

	XXX map[string]interface{} `yaml:",inline"`
}

// Target represents Azure target resource and its associated metric definitions
type Resource struct {
	Name         string   `yaml:"name"`
	Metrics      []string `yaml:"metrics"`
	Aggregations []string `yaml:"aggregations"`

	XXX map[string]interface{} `yaml:",inline"`
}

// Target represents Azure target resource and its associated metric definitions
type ResourceGroup struct {
	Name            string   `yaml:"name"`
	ResourceTypes   []string `yaml:"resource_types"`
	ResourceInclude []string `yaml:"resource_include"`
	ResourceExclude []string `yaml:"resource_exclude"`
	Metrics         []string `yaml:"metrics"`
	Aggregations    []string `yaml:"aggregations"`

	XXX map[string]interface{} `yaml:",inline"`
}

func checkOverflow(m map[string]interface{}, ctx string) error {
	if len(m) > 0 {
		var keys []string
		for k := range m {
			keys = append(keys, k)
		}
		return fmt.Errorf("unknown fields in %s: %s", ctx, strings.Join(keys, ", "))
	}
	return nil
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (s *Config) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain Config
	if err := unmarshal((*plain)(s)); err != nil {
		return err
	}
	if err := checkOverflow(s.XXX, "config"); err != nil {
		return err
	}
	return nil
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (s *Credentials) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain Credentials
	if err := unmarshal((*plain)(s)); err != nil {
		return err
	}
	if err := checkOverflow(s.XXX, "config"); err != nil {
		return err
	}
	return nil
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (s *Resource) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain Resource
	if err := unmarshal((*plain)(s)); err != nil {
		return err
	}
	if err := checkOverflow(s.XXX, "config"); err != nil {
		return err
	}
	return nil
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (s *ResourceGroup) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain ResourceGroup
	if err := unmarshal((*plain)(s)); err != nil {
		return err
	}
	if err := checkOverflow(s.XXX, "config"); err != nil {
		return err
	}
	return nil
}

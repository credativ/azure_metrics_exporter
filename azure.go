package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// AzureMetricDefinitionResponse represents metric definition response for a given resource from Azure.
type AzureMetricDefinitionResponse struct {
	MetricDefinitionResponses []metricDefinitionResponse `json:"value"`
}
type metricDefinitionResponse struct {
	Dimensions []struct {
		LocalizedValue string `json:"localizedValue"`
		Value          string `json:"value"`
	} `json:"dimensions"`
	ID                   string `json:"id"`
	IsDimensionRequired  bool   `json:"isDimensionRequired"`
	MetricAvailabilities []struct {
		Retention string `json:"retention"`
		TimeGrain string `json:"timeGrain"`
	} `json:"metricAvailabilities"`
	Name struct {
		LocalizedValue string `json:"localizedValue"`
		Value          string `json:"value"`
	} `json:"name"`
	PrimaryAggregationType string `json:"primaryAggregationType"`
	ResourceID             string `json:"resourceId"`
	Unit                   string `json:"unit"`
}

// AzureMetricValueResponse represents a metric value response for a given metric definition.
type AzureMetricValueResponse struct {
	Value []struct {
		Timeseries []struct {
			Data []struct {
				TimeStamp string  `json:"timeStamp"`
				Total     float64 `json:"total"`
				Average   float64 `json:"average"`
				Minimum   float64 `json:"minimum"`
				Maximum   float64 `json:"maximum"`
			} `json:"data"`
		} `json:"timeseries"`
		ID   string `json:"id"`
		Name struct {
			LocalizedValue string `json:"localizedValue"`
			Value          string `json:"value"`
		} `json:"name"`
		Type string `json:"type"`
		Unit string `json:"unit"`
	} `json:"value"`
	APIError struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type AzureResourceListResponse struct {
	Value []struct {
		Id        string `json:"id"`
		Name      string `json:"name"`
		Type      string `json:"type"`
		ManagedBy string `json:"managedBy"`
		Location  string `json:"location"`
	} `json:"value"`
}

// AzureClient represents our client to talk to the Azure api
type AzureClient struct {
	client               *http.Client
	accessToken          string
	accessTokenExpiresOn time.Time
}

// NewAzureClient returns an Azure client to talk the Azure API
func NewAzureClient() *AzureClient {
	return &AzureClient{
		client:               &http.Client{},
		accessToken:          "",
		accessTokenExpiresOn: time.Time{},
	}
}

func (ac *AzureClient) getAccessToken() error {
	target := fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/token", sc.C.Credentials.TenantID)
	form := url.Values{
		"grant_type":    {"client_credentials"},
		"resource":      {"https://management.azure.com/"},
		"client_id":     {sc.C.Credentials.ClientID},
		"client_secret": {sc.C.Credentials.ClientSecret},
	}
	resp, err := ac.client.PostForm(target, form)
	if err != nil {
		return fmt.Errorf("Error authenticating against Azure API: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("Did not get status code 200, got: %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("Error reading body of response: %v", err)
	}
	var data map[string]interface{}
	err = json.Unmarshal(body, &data)
	if err != nil {
		return fmt.Errorf("Error unmarshalling response body: %v", err)
	}
	ac.accessToken = data["access_token"].(string)
	expiresOn, err := strconv.ParseInt(data["expires_on"].(string), 10, 64)
	if err != nil {
		return fmt.Errorf("Error ParseInt of expires_on failed: %v", err)
	}
	ac.accessTokenExpiresOn = time.Unix(expiresOn, 0).UTC()

	return nil
}

// Loop through all specified resource targets and get their respective metric definitions.
func (ac *AzureClient) getMetricDefinitions() (map[string]AzureMetricDefinitionResponse, error) {
	apiVersion := "2018-01-01"
	definitions := make(map[string]AzureMetricDefinitionResponse)

	for _, target := range sc.C.Targets {
		metricsResource := fmt.Sprintf("subscriptions/%s%s", sc.C.Credentials.SubscriptionID, target.Resource)
		metricsTarget := fmt.Sprintf("https://management.azure.com/%s/providers/microsoft.insights/metricDefinitions?api-version=%s", metricsResource, apiVersion)
		req, err := http.NewRequest("GET", metricsTarget, nil)
		if err != nil {
			return nil, fmt.Errorf("Error creating HTTP request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+ac.accessToken)
		resp, err := ac.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("Error: %v", err)
		}
		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("Error reading body of response: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("Error: %v", string(body))
		}

		def := AzureMetricDefinitionResponse{}
		err = json.Unmarshal(body, &def)
		if err != nil {
			return nil, fmt.Errorf("Error unmarshalling response body: %v", err)
		}
		definitions[target.Resource] = def
	}
	return definitions, nil
}

func (ac *AzureClient) getMetricValue(resource string, metricNames string, aggregations []string) (AzureMetricValueResponse, error) {
	apiVersion := "2018-01-01"
	now := time.Now().UTC()
	refreshAt := ac.accessTokenExpiresOn.Add(-10 * time.Minute)
	if now.After(refreshAt) {
		err := ac.getAccessToken()
		if err != nil {
			return AzureMetricValueResponse{}, fmt.Errorf("Error refreshing access token: %v", err)
		}
	}

	metricsResource := fmt.Sprintf("subscriptions/%s%s", sc.C.Credentials.SubscriptionID, resource)
	endTime, startTime := GetTimes()

	metricValueEndpoint := fmt.Sprintf("https://management.azure.com/%s/providers/microsoft.insights/metrics", metricsResource)

	req, err := http.NewRequest("GET", metricValueEndpoint, nil)
	if err != nil {
		return AzureMetricValueResponse{}, fmt.Errorf("Error creating HTTP request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+ac.accessToken)

	values := url.Values{}
	if metricNames != "" {
		values.Add("metricnames", metricNames)
	}
	if len(aggregations) > 0 {
		values.Add("aggregation", strings.Join(aggregations, ","))
	} else {
		values.Add("aggregation", "Total,Average,Minimum,Maximum")
	}
	values.Add("timespan", fmt.Sprintf("%s/%s", startTime, endTime))
	values.Add("api-version", apiVersion)

	req.URL.RawQuery = values.Encode()

	log.Printf("GET %s", req.URL)
	resp, err := ac.client.Do(req)
	if err != nil {
		return AzureMetricValueResponse{}, fmt.Errorf("Error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return AzureMetricValueResponse{}, fmt.Errorf("Unable to query metrics API with status code: %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return AzureMetricValueResponse{}, fmt.Errorf("Error reading body of response: %v", err)
	}

	var data AzureMetricValueResponse
	err = json.Unmarshal(body, &data)
	if err != nil {
		return AzureMetricValueResponse{}, fmt.Errorf("Error unmarshalling response body: %v", err)
	}

	return data, nil
}

func (ac *AzureClient) listFromResourceGroup(resourceGroup string, resourceTypes []string) ([]string, error) {
	apiVersion := "2018-02-01"
	now := time.Now().UTC()
	refreshAt := ac.accessTokenExpiresOn.Add(-10 * time.Minute)
	if now.After(refreshAt) {
		err := ac.getAccessToken()
		if err != nil {
			return nil, fmt.Errorf("Error refreshing access token: %v", err)
		}
	}

	var filterTypesElements []string
	for _, filterType := range resourceTypes {
		filterTypesElements = append(filterTypesElements, fmt.Sprintf("resourcetype eq '%s'", filterType))
	}
	filterTypes := url.QueryEscape(strings.Join(filterTypesElements, " or "))

	subscription := fmt.Sprintf("subscriptions/%s", sc.C.Credentials.SubscriptionID)

	metricValueEndpoint := fmt.Sprintf("https://management.azure.com/%s/resourceGroups/%s/resources?api-version=%s&$filter=%s", subscription, resourceGroup, apiVersion, filterTypes)

	req, err := http.NewRequest("GET", metricValueEndpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("Error creating HTTP request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+ac.accessToken)

	log.Printf("GET %s", req.URL)

	resp, err := ac.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Unable to query resource group API with status code: %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("Error reading body of response: %v", err)
	}

	var data AzureResourceListResponse
	err = json.Unmarshal(body, &data)
	if err != nil {
		return nil, fmt.Errorf("Error unmarshalling response body: %v", err)
	}

	var resources []string

	for _, result := range data.Value {
		// subscription + leading '/'
		subscriotionPrefixLen := len(subscription) + 1

		resources = append(resources, result.Id[subscriotionPrefixLen:])
	}

	return resources, nil
}

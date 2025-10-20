package openMeteo

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type Response struct {
	Current struct {
		Time          string  `json:"time"`
		Temperature2m float64 `json:"temperature_2m"`
	}
}

type client struct {
	httpClient *http.Client
}

func NewClient(httpClient *http.Client) *client {
	return &client{
		httpClient: httpClient,
	}
}

func (c *client) GetTemperature(latitude, longitude float64) (Response, error) {
	res, err := http.Get(fmt.Sprintf("https://api.open-meteo.com/v1/forecast?latitude=%f&longitude=%f&current=temperature_2m",
		latitude,
		longitude,
	),
	)
	if err != nil {
		return Response{}, err
	}
	if res.StatusCode != http.StatusOK {
		return Response{}, fmt.Errorf("status code: %d", res.StatusCode)
	}

	defer res.Body.Close()

	var weatherResp Response

	err = json.NewDecoder(res.Body).Decode(&weatherResp)

	if err != nil {
		return Response{}, fmt.Errorf("decoding response body: %w", err)
	}

	return weatherResp, nil
}

package etherfi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/D8-X/d8x-etherfi/internal/utils"
)

// flipsideGetAddresses returns all addresses that have received the
// pool-token within the specified blocks, inclusive [fromBlockNr, toBlockNr]
func (app *App) flipsideGetAddresses(fromBlockNr, toBlockNr int64) (*utils.FSResultSet, error) {
	fmt.Printf("Flipside query blocks (%d, %d]", fromBlockNr, toBlockNr)
	var err error
	var id string
	query := fmt.Sprintf(
		`SELECT distinct(to_address) FROM arbitrum.core.ez_token_transfers WHERE lower(contract_address)='%s' AND block_number>=%d AND block_number<=%d`,
		strings.ToLower(app.PoolShareTknAddr.Hex()), fromBlockNr, toBlockNr)
	id, err = createQueryRun(query, app.FlipsideKey)
	if err != nil {
		return nil, errors.New("creating flipside query run:" + err.Error())
	}
	var status string
	for {
		status, err = queryStatus(id, app.FlipsideKey)
		if err != nil {
			return nil, errors.New("could not retrieve query status:" + err.Error())
		}
		if status != "QUERY_STATE_READY" && status != "QUERY_STATE_STREAMING_RESULTS" && status != "QUERY_STATE_RUNNING" {
			break
		}
		time.Sleep(1 * time.Second)
	}
	if status != "QUERY_STATE_SUCCESS" {
		return nil, errors.New(status)
	}
	fsSet, err := queryResults(id, app.FlipsideKey)
	if err != nil {
		return nil, errors.New("queryResults:" + err.Error())
	}
	return fsSet, nil
}

func queryResults(queryRunId, key string) (*utils.FSResultSet, error) {
	url := "https://api-v2.flipsidecrypto.xyz/json-rpc"
	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "getQueryRunResults",
		"params": []map[string]interface{}{
			{
				"queryRunId": queryRunId,
				"format":     "csv",
				"page": map[string]int{
					"number": 1,
					"size":   5000,
				},
			},
		},
		"id": 1,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, errors.New("error encoding JSON:" + err.Error())
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, errors.New("error creating request:" + err.Error())
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", key)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.New("error sending request:" + err.Error())
	}
	defer resp.Body.Close()
	if resp.Status != "200 OK" {
		return nil, errors.New("response status: " + resp.Status)
	}

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return nil, errors.New("error decoding response: " + resp.Status)
	}
	var fsSet utils.FSResultSet
	jsonBytes, err := json.Marshal(result["result"])
	if err != nil {
		return nil, errors.New("error stringifying result: " + resp.Status)
	}
	err = json.Unmarshal(jsonBytes, &fsSet)
	return &fsSet, err
}

func queryStatus(queryRunId, key string) (string, error) {
	url := "https://api-v2.flipsidecrypto.xyz/json-rpc"
	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "getQueryRun",
		"params": []map[string]string{
			{
				"queryRunId": queryRunId,
			},
		},
		"id": 1,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", key)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.Status != "200 OK" {
		return "", errors.New("Response status: " + resp.Status)
	}
	fmt.Println("Response Status:", resp.Status)

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return "", err
	}

	r := result["result"].(map[string]interface{})
	queryRun, ok := r["queryRun"].(map[string]interface{})
	if !ok {
		return "", err
	}
	state, ok := queryRun["state"].(string)
	if !ok {
		return "", err
	}
	return state, nil
}

func createQueryRun(query, key string) (string, error) {
	url := "https://api-v2.flipsidecrypto.xyz/json-rpc"
	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "createQueryRun",
		"params": []map[string]interface{}{
			{
				"resultTTLHours": 1,
				"maxAgeMinutes":  0,
				"sql":            query,
				"tags": map[string]string{
					"source": "postman-demo",
					"env":    "test",
				},
				"dataSource":   "snowflake-default",
				"dataProvider": "flipside",
			},
		},
		"id": 1,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", key)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	if resp.Status != "200 OK" {
		return "", errors.New("Response status: " + resp.Status)
	}
	defer resp.Body.Close()
	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return "", err
	}
	r := result["result"].(map[string]interface{})
	queryRequest, ok := r["queryRequest"].(map[string]interface{})
	if !ok {
		return "", err
	}
	id, ok := queryRequest["queryRunId"].(string)
	if !ok {
		return "", err
	}
	return id, nil
}

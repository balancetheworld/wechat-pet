package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	askapp "github.com/balancetheworld/wechat-pet/internal/app/ask"
)

func TestOpenAIExecutorReturnsCompletedAnalysis(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/responses" || request.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("request = %s %s, authorization = %s", request.Method, request.URL.Path, request.Header.Get("Authorization"))
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), "family-secret") || strings.Contains(string(body), "run-secret") || strings.Contains(string(body), "pet-secret") {
			t.Fatalf("request contains internal identifiers: %s", body)
		}
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatal(err)
		}
		text, ok := payload["text"].(map[string]any)
		if !ok {
			t.Fatalf("text config = %#v", payload["text"])
		}
		format, ok := text["format"].(map[string]any)
		if !ok || format["type"] != "json_schema" || format["strict"] != true {
			t.Fatalf("response format = %#v", text["format"])
		}
		writeOpenAIResponse(t, response, analysisOutput{
			Status:               string(askapp.RunCompleted),
			RiskLevel:            string(askapp.RiskYellow),
			CurrentAssessment:    "目前需要密切观察",
			Observations:         []string{"今天出现两次呕吐"},
			PossibleCauses:       []string{"饮食变化"},
			HomeActions:          []string{"少量提供清水并记录状态"},
			EscalationConditions: []string{"持续呕吐或精神变差时就医"},
		})
	}))
	defer server.Close()
	executor := newTestOpenAIExecutor(t, server.URL+"/v1", time.Second)
	decision, err := executor.Execute(context.Background(), askapp.RunInput{
		Session: askapp.Session{ID: "family-secret"},
		Turn:    askapp.Turn{ID: "run-secret", Input: "团子今天吐了两次"},
		Run:     askapp.Run{ID: "run-secret"},
		Context: askapp.ContextSnapshot{Pet: askapp.PetContext{ID: "pet-secret", Name: "团子"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Status != askapp.RunCompleted || decision.RiskLevel != askapp.RiskYellow || decision.Data["current_assessment"] != "目前需要密切观察" {
		t.Fatalf("decision = %+v", decision)
	}
}

func TestOpenAIExecutorReturnsClarificationQuestion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		writeOpenAIResponse(t, response, analysisOutput{Status: string(askapp.RunWaitingInput), RiskLevel: string(askapp.RiskUnknown), Question: "症状从什么时候开始？"})
	}))
	defer server.Close()
	executor := newTestOpenAIExecutor(t, server.URL+"/v1", time.Second)
	decision, err := executor.Execute(context.Background(), askapp.RunInput{Turn: askapp.Turn{Input: "宠物不舒服"}})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Status != askapp.RunWaitingInput || decision.EventType != "assistant.question" || decision.Data["question"] != "症状从什么时候开始？" {
		t.Fatalf("decision = %+v", decision)
	}
}

func TestOpenAIExecutorStreamsSafeDeltas(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "text/event-stream")
		response.WriteHeader(http.StatusOK)
		output, err := json.Marshal(analysisOutput{Status: string(askapp.RunCompleted), RiskLevel: string(askapp.RiskYellow), CurrentAssessment: "需要观察", Observations: []string{"精神尚可"}, PossibleCauses: []string{"饮食变化"}, HomeActions: []string{"记录状态"}, EscalationConditions: []string{"持续加重时就医"}})
		if err != nil {
			t.Fatal(err)
		}
		payload, err := json.Marshal(map[string]any{"type": "response.completed", "response": map[string]any{"id": "resp-test", "object": "response", "created_at": 1, "status": "completed", "model": "test-model", "output": []map[string]any{{"id": "msg-test", "type": "message", "status": "completed", "role": "assistant", "content": []map[string]any{{"type": "output_text", "text": string(output), "annotations": []any{}}}}}, "usage": map[string]any{"input_tokens": 12, "output_tokens": 8, "total_tokens": 20}}})
		if err != nil {
			t.Fatal(err)
		}
		_, _ = response.Write([]byte("event: response.completed\ndata: " + string(payload) + "\n\n"))
	}))
	defer server.Close()
	executor := newTestOpenAIExecutor(t, server.URL+"/v1", time.Second)
	deltas := make([]string, 0)
	decision, err := executor.ExecuteStream(context.Background(), askapp.RunInput{Turn: askapp.Turn{Input: "问题"}}, func(delta string) error {
		deltas = append(deltas, delta)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Status != askapp.RunCompleted || strings.Join(deltas, "") != "需要观察" {
		t.Fatalf("decision = %+v, deltas = %q", decision, strings.Join(deltas, ""))
	}
}

func TestOpenAIExecutorReportsSafeObservation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		writeOpenAITextResponse(t, response, `{"status":"waiting_input","risk_level":"unknown","question":"症状从什么时候开始？","current_assessment":"","observations":[],"possible_causes":[],"home_actions":[],"escalation_conditions":[]}`)
	}))
	defer server.Close()
	observations := make(chan OpenAIObservation, 1)
	executor, err := NewOpenAIExecutor(OpenAIConfig{APIKey: "test-key", BaseURL: server.URL + "/v1", Model: "test-model", Timeout: time.Second, Observer: func(value OpenAIObservation) { observations <- value }})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := executor.Execute(context.Background(), askapp.RunInput{Turn: askapp.Turn{Input: "问题"}}); err != nil {
		t.Fatal(err)
	}
	select {
	case observation := <-observations:
		if observation.Model != "test-model" || observation.Status != string(askapp.RunWaitingInput) || observation.TotalTokens != 20 || observation.Duration <= 0 {
			t.Fatalf("observation = %+v", observation)
		}
	case <-time.After(time.Second):
		t.Fatal("observation callback was not called")
	}
}

func TestOpenAIExecutorClassifiesAPIErrors(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		code      string
		retryable bool
	}{
		{name: "rate limited", status: http.StatusTooManyRequests, code: "provider_rate_limited", retryable: true},
		{name: "unavailable", status: http.StatusServiceUnavailable, code: "provider_unavailable", retryable: true},
		{name: "bad request", status: http.StatusBadRequest, code: "provider_request_invalid", retryable: false},
		{name: "unauthorized", status: http.StatusUnauthorized, code: "provider_auth_failed", retryable: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
				response.Header().Set("Content-Type", "application/json")
				response.Header().Set("Retry-After", "7")
				response.WriteHeader(test.status)
				_, _ = response.Write([]byte(`{"error":{"message":"failed","type":"api_error","code":"failed","param":null}}`))
			}))
			defer server.Close()
			executor := newTestOpenAIExecutor(t, server.URL+"/v1", time.Second)
			_, err := executor.Execute(context.Background(), askapp.RunInput{Turn: askapp.Turn{Input: "问题"}})
			code, retryable := askapp.ExecutorErrorDetails(err)
			if code != test.code || retryable != test.retryable {
				t.Fatalf("error = %v, code = %s, retryable = %t", err, code, retryable)
			}
			if test.status == http.StatusTooManyRequests && askapp.ExecutorRetryDelay(err, time.Second) != 7*time.Second {
				t.Fatalf("retry delay = %s", askapp.ExecutorRetryDelay(err, time.Second))
			}
		})
	}
}

func TestOpenAIExecutorReportsClassifiedError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusTooManyRequests)
		_, _ = response.Write([]byte(`{"error":{"message":"rate limited"}}`))
	}))
	defer server.Close()
	observations := make(chan OpenAIObservation, 1)
	executor := newObservedTestOpenAIExecutor(t, server.URL+"/v1", observations)
	_, err := executor.Execute(context.Background(), askapp.RunInput{Turn: askapp.Turn{Input: "问题"}})
	if code, retryable := askapp.ExecutorErrorDetails(err); code != "provider_rate_limited" || !retryable {
		t.Fatalf("error = %v, code = %s, retryable = %t", err, code, retryable)
	}
	select {
	case observation := <-observations:
		if observation.ErrorCode != "provider_rate_limited" || !observation.Retryable || observation.Status != "error" || observation.Model != "test-model" {
			t.Fatalf("observation = %+v", observation)
		}
	case <-time.After(time.Second):
		t.Fatal("observation callback was not called")
	}
}

func TestOpenAIExecutorClassifiesTimeoutAndInvalidOutput(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
			time.Sleep(100 * time.Millisecond)
			response.WriteHeader(http.StatusOK)
		}))
		defer server.Close()
		executor := newTestOpenAIExecutor(t, server.URL+"/v1", 10*time.Millisecond)
		_, err := executor.Execute(context.Background(), askapp.RunInput{Turn: askapp.Turn{Input: "问题"}})
		code, retryable := askapp.ExecutorErrorDetails(err)
		if code != "provider_timeout" || !retryable {
			t.Fatalf("error = %v, code = %s, retryable = %t", err, code, retryable)
		}
	})
	t.Run("invalid output", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
			writeOpenAITextResponse(t, response, "not-json")
		}))
		defer server.Close()
		executor := newTestOpenAIExecutor(t, server.URL+"/v1", time.Second)
		_, err := executor.Execute(context.Background(), askapp.RunInput{Turn: askapp.Turn{Input: "问题"}})
		code, retryable := askapp.ExecutorErrorDetails(err)
		if code != "provider_output_invalid" || retryable {
			t.Fatalf("error = %v, code = %s, retryable = %t", err, code, retryable)
		}
	})
}

func TestNewOpenAIExecutorValidatesConfiguration(t *testing.T) {
	valid := OpenAIConfig{APIKey: "key", Model: "model", Timeout: time.Second}
	if _, err := NewOpenAIExecutor(OpenAIConfig{Model: "model", Timeout: time.Second}); err == nil {
		t.Fatal("missing API key error = nil")
	}
	if _, err := NewOpenAIExecutor(OpenAIConfig{APIKey: "key", Timeout: time.Second}); err == nil {
		t.Fatal("missing model error = nil")
	}
	invalidURL := valid
	invalidURL.BaseURL = "://invalid"
	if _, err := NewOpenAIExecutor(invalidURL); err == nil {
		t.Fatal("invalid base URL error = nil")
	}
	if _, err := NewOpenAIExecutor(valid); err != nil {
		t.Fatal(err)
	}
}

func newTestOpenAIExecutor(t *testing.T, baseURL string, timeout time.Duration) *OpenAIExecutor {
	t.Helper()
	executor, err := NewOpenAIExecutor(OpenAIConfig{APIKey: "test-key", BaseURL: baseURL, Model: "test-model", Timeout: timeout})
	if err != nil {
		t.Fatal(err)
	}
	return executor
}

func newObservedTestOpenAIExecutor(t *testing.T, baseURL string, observations chan<- OpenAIObservation) *OpenAIExecutor {
	t.Helper()
	executor, err := NewOpenAIExecutor(OpenAIConfig{APIKey: "test-key", BaseURL: baseURL, Model: "test-model", Timeout: time.Second, Observer: func(value OpenAIObservation) { observations <- value }})
	if err != nil {
		t.Fatal(err)
	}
	return executor
}

func writeOpenAIResponse(t *testing.T, response http.ResponseWriter, output analysisOutput) {
	t.Helper()
	data, err := json.Marshal(output)
	if err != nil {
		t.Fatal(err)
	}
	writeOpenAITextResponse(t, response, string(data))
}

func writeOpenAITextResponse(t *testing.T, response http.ResponseWriter, text string) {
	t.Helper()
	response.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(response).Encode(map[string]any{
		"id":         "resp-test",
		"object":     "response",
		"created_at": 1,
		"status":     "completed",
		"model":      "test-model",
		"output": []map[string]any{{
			"id": "msg-test", "type": "message", "status": "completed", "role": "assistant",
			"content": []map[string]any{{"type": "output_text", "text": text, "annotations": []any{}}},
		}},
		"usage": map[string]any{"input_tokens": 12, "output_tokens": 8, "total_tokens": 20},
	}); err != nil {
		t.Fatal(err)
	}
}

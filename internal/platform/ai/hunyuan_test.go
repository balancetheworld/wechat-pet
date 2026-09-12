package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	askapp "github.com/balancetheworld/wechat-pet/internal/app/ask"
)

func TestHunyuanExecutorUsesChatCompletions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/chat/completions" || request.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("request = %s %s, authorization = %s", request.Method, request.URL.Path, request.Header.Get("Authorization"))
		}
		response.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(response).Encode(map[string]any{
			"id": "chat-test", "object": "chat.completion", "created": 1, "model": "hunyuan-turbos-latest",
			"choices": []map[string]any{{"index": 0, "finish_reason": "stop", "message": map[string]any{"role": "assistant", "content": `{"status":"waiting_input","risk_level":"unknown","question":"症状从什么时候开始？","current_assessment":"","observations":[],"possible_causes":[],"home_actions":[],"escalation_conditions":[]}`}}},
			"usage":   map[string]any{"prompt_tokens": 12, "completion_tokens": 8, "total_tokens": 20},
		})
	}))
	defer server.Close()
	executor, err := NewHunyuanExecutor(OpenAIConfig{APIKey: "test-key", BaseURL: server.URL + "/v1", Model: "hunyuan-turbos-latest", Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	decision, err := executor.Execute(context.Background(), askapp.RunInput{Turn: askapp.Turn{Input: "宠物不舒服"}})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Status != askapp.RunWaitingInput || decision.Data["question"] != "症状从什么时候开始？" {
		t.Fatalf("decision = %+v", decision)
	}
}

func TestHunyuanExecutorRoutesIntentWithChatCompletions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/chat/completions" || request.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("request = %s %s, authorization = %s", request.Method, request.URL.Path, request.Header.Get("Authorization"))
		}
		response.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(response).Encode(map[string]any{
			"id": "chat-route", "object": "chat.completion", "created": 1, "model": "hunyuan-turbos-latest",
			"choices": []map[string]any{{"index": 0, "finish_reason": "stop", "message": map[string]any{"role": "assistant", "content": `{"intent":"casual_chat","reply":"你好，需要我做什么？","question":""}`}}},
			"usage":   map[string]any{"prompt_tokens": 10, "completion_tokens": 6, "total_tokens": 16},
		})
	}))
	defer server.Close()
	executor, err := NewHunyuanExecutor(OpenAIConfig{APIKey: "test-key", BaseURL: server.URL + "/v1", Model: "hunyuan-turbos-latest", Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	decision, err := executor.Route(context.Background(), askapp.IntentInput{Turn: askapp.Turn{Input: "你好呀"}})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Intent != askapp.IntentCasualChat || decision.Reply != "你好，需要我做什么？" {
		t.Fatalf("decision = %+v", decision)
	}
}

func TestHunyuanExecutorStreamsStructuredOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "text/event-stream")
		response.WriteHeader(http.StatusOK)
		chunks := []string{`{"status":"completed","risk_level":"yellow","current_assessment":"需要观察","observations":["精神尚可"],"possible_causes":["饮食变化"],"home_actions":["记录状态"],"escalation_conditions":["持续加重时就医"]}`}
		for _, content := range chunks {
			data := map[string]any{"id": "chat-test", "object": "chat.completion.chunk", "created": 1, "model": "hunyuan-turbos-latest", "choices": []map[string]any{{"index": 0, "delta": map[string]string{"role": "assistant", "content": content}, "finish_reason": ""}}}
			payload, _ := json.Marshal(data)
			_, _ = response.Write([]byte("data: " + string(payload) + "\n\n"))
		}
		_, _ = response.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()
	executor, err := NewHunyuanExecutor(OpenAIConfig{APIKey: "test-key", BaseURL: server.URL + "/v1", Model: "hunyuan-turbos-latest", Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	var deltas []string
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

func TestHunyuanExecutorTreatsQuotaExhaustionAsPermanent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(http.StatusTooManyRequests)
		_, _ = response.Write([]byte(`{"code":"EXCEED_TOKEN_QUOTA_LIMIT","message":"Token usage exceeded quota limit"}`))
	}))
	defer server.Close()
	executor, err := NewHunyuanExecutor(OpenAIConfig{APIKey: "test-key", BaseURL: server.URL + "/v1", Model: "hunyuan-turbos-latest", Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	_, err = executor.Execute(context.Background(), askapp.RunInput{Turn: askapp.Turn{Input: "问题"}})
	code, retryable := askapp.ExecutorErrorDetails(err)
	if code != "provider_quota_exhausted" || retryable {
		t.Fatalf("error = %v, code = %s, retryable = %t", err, code, retryable)
	}
}

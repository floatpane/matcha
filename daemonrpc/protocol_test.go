package daemonrpc

import (
	"encoding/json"
	"testing"
)

func TestDecodeMessage_Request(t *testing.T) {
	raw := json.RawMessage(`{"id":1,"method":"Ping"}`)
	msg, err := DecodeMessage(raw)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Request == nil {
		t.Fatal("expected Request, got nil")
	}
	if msg.Request.Method != "Ping" {
		t.Errorf("method = %q, want Ping", msg.Request.Method)
	}
	if msg.Request.ID != 1 {
		t.Errorf("id = %d, want 1", msg.Request.ID)
	}
	if msg.Response != nil || msg.Event != nil {
		t.Error("expected only Request to be set")
	}
}

func TestDecodeMessage_Response(t *testing.T) {
	raw := json.RawMessage(`{"id":1,"result":{"pong":true}}`)
	msg, err := DecodeMessage(raw)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Response == nil {
		t.Fatal("expected Response, got nil")
	}
	if msg.Response.ID != 1 {
		t.Errorf("id = %d, want 1", msg.Response.ID)
	}
	if msg.Response.Error != nil {
		t.Error("expected no error")
	}
}

func TestDecodeMessage_ResponseError(t *testing.T) {
	raw := json.RawMessage(`{"id":2,"error":{"code":-32601,"message":"not found"}}`)
	msg, err := DecodeMessage(raw)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Response == nil {
		t.Fatal("expected Response")
	}
	if msg.Response.Error == nil {
		t.Fatal("expected error in response")
	}
	if msg.Response.Error.Code != ErrCodeNotFound {
		t.Errorf("code = %d, want %d", msg.Response.Error.Code, ErrCodeNotFound)
	}
	if msg.Response.Error.Message != "not found" {
		t.Errorf("message = %q, want 'not found'", msg.Response.Error.Message)
	}
}

func TestDecodeMessage_Event(t *testing.T) {
	raw := json.RawMessage(`{"type":"NewMail","data":{"account_id":"abc","folder":"INBOX"}}`)
	msg, err := DecodeMessage(raw)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Event == nil {
		t.Fatal("expected Event, got nil")
	}
	if msg.Event.Type != EventNewMail {
		t.Errorf("type = %q, want NewMail", msg.Event.Type)
	}

	var ev NewMailEvent
	if err := json.Unmarshal(msg.Event.Data, &ev); err != nil {
		t.Fatal(err)
	}
	if ev.AccountID != "abc" {
		t.Errorf("account_id = %q, want abc", ev.AccountID)
	}
}

func TestDecodeMessage_Invalid(t *testing.T) {
	raw := json.RawMessage(`{invalid}`)
	_, err := DecodeMessage(raw)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestError_ErrorInterface(t *testing.T) {
	e := &Error{Code: ErrCodeInternal, Message: "something broke"}
	if e.Error() != "something broke" {
		t.Errorf("Error() = %q, want 'something broke'", e.Error())
	}
}

func TestRequestRoundTrip(t *testing.T) {
	params, _ := json.Marshal(FetchEmailsParams{
		AccountID: "acc1",
		Folder:    "INBOX",
		Limit:     50,
		Offset:    0,
	})
	req := Request{
		ID:     42,
		Method: MethodFetchEmails,
		Params: params,
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}

	msg, err := DecodeMessage(data)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Request == nil {
		t.Fatal("expected Request")
	}
	if msg.Request.ID != 42 {
		t.Errorf("id = %d, want 42", msg.Request.ID)
	}

	var p FetchEmailsParams
	if err := json.Unmarshal(msg.Request.Params, &p); err != nil {
		t.Fatal(err)
	}
	if p.AccountID != "acc1" || p.Folder != "INBOX" || p.Limit != 50 {
		t.Errorf("params mismatch: %+v", p)
	}
}

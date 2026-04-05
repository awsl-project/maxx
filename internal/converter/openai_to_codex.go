package converter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func init() {
	RegisterConverter(domain.ClientTypeOpenAI, domain.ClientTypeCodex, &openaiToCodexRequest{}, &openaiToCodexResponse{})
}

type openaiToCodexRequest struct{}
type openaiToCodexResponse struct{}

func (c *openaiToCodexRequest) Transform(body []byte, model string, stream bool) ([]byte, error) {
	var tmp interface{}
	if err := json.Unmarshal(body, &tmp); err != nil {
		return nil, err
	}
	rawJSON := bytes.Clone(body)
	out := `{"instructions":""}`

	out, _ = sjson.Set(out, "stream", stream)

	if v := gjson.GetBytes(rawJSON, "reasoning_effort"); v.Exists() {
		out, _ = sjson.Set(out, "reasoning.effort", v.Value())
	} else {
		out, _ = sjson.Set(out, "reasoning.effort", "medium")
	}
	if v := gjson.GetBytes(rawJSON, "service_tier"); v.Exists() && v.String() != "" {
		out, _ = sjson.Set(out, "service_tier", v.Value())
	}
	out, _ = sjson.Set(out, "parallel_tool_calls", true)
	out, _ = sjson.Set(out, "reasoning.summary", "auto")
	out, _ = sjson.Set(out, "include", []string{"reasoning.encrypted_content"})

	out, _ = sjson.Set(out, "model", model)

	originalToolNameMap := map[string]string{}
	if tools := gjson.GetBytes(rawJSON, "tools"); tools.IsArray() && len(tools.Array()) > 0 {
		var names []string
		for _, t := range tools.Array() {
			if t.Get("type").String() == "function" {
				if v := t.Get("function.name"); v.Exists() {
					names = append(names, v.String())
				}
			}
		}
		if len(names) > 0 {
			originalToolNameMap = buildShortNameMap(names)
		}
	}

	out, _ = sjson.SetRaw(out, "input", `[]`)
	if messages := gjson.GetBytes(rawJSON, "messages"); messages.IsArray() {
		for _, m := range messages.Array() {
			role := m.Get("role").String()
			switch role {
			case "tool":
				funcOutput := `{}`
				funcOutput, _ = sjson.Set(funcOutput, "type", "function_call_output")
				funcOutput, _ = sjson.Set(funcOutput, "call_id", m.Get("tool_call_id").String())
				funcOutput, _ = sjson.Set(funcOutput, "output", m.Get("content").String())
				out, _ = sjson.SetRaw(out, "input.-1", funcOutput)
			default:
				msg := `{}`
				msg, _ = sjson.Set(msg, "type", "message")
				if role == "system" {
					msg, _ = sjson.Set(msg, "role", "developer")
				} else {
					msg, _ = sjson.Set(msg, "role", role)
				}
				msg, _ = sjson.SetRaw(msg, "content", `[]`)

				c := m.Get("content")
				if c.Exists() && c.Type == gjson.String && c.String() != "" {
					partType := "input_text"
					if role == "assistant" {
						partType = "output_text"
					}
					part := `{}`
					part, _ = sjson.Set(part, "type", partType)
					part, _ = sjson.Set(part, "text", c.String())
					msg, _ = sjson.SetRaw(msg, "content.-1", part)
				} else if c.Exists() && c.IsArray() {
					for _, it := range c.Array() {
						t := it.Get("type").String()
						switch t {
						case "text":
							partType := "input_text"
							if role == "assistant" {
								partType = "output_text"
							}
							part := `{}`
							part, _ = sjson.Set(part, "type", partType)
							part, _ = sjson.Set(part, "text", it.Get("text").String())
							msg, _ = sjson.SetRaw(msg, "content.-1", part)
						case "image_url":
							if role == "user" {
								part := `{}`
								part, _ = sjson.Set(part, "type", "input_image")
								if u := it.Get("image_url.url"); u.Exists() {
									part, _ = sjson.Set(part, "image_url", u.String())
								}
								msg, _ = sjson.SetRaw(msg, "content.-1", part)
							}
						}
					}
				}

				out, _ = sjson.SetRaw(out, "input.-1", msg)

				if role == "assistant" {
					if toolCalls := m.Get("tool_calls"); toolCalls.Exists() && toolCalls.IsArray() {
						for _, tc := range toolCalls.Array() {
							if tc.Get("type").String() != "function" {
								continue
							}
							funcCall := `{}`
							funcCall, _ = sjson.Set(funcCall, "type", "function_call")
							funcCall, _ = sjson.Set(funcCall, "call_id", tc.Get("id").String())
							name := tc.Get("function.name").String()
							if short, ok := originalToolNameMap[name]; ok {
								name = short
							} else {
								name = shortenNameIfNeeded(name)
							}
							funcCall, _ = sjson.Set(funcCall, "name", name)
							funcCall, _ = sjson.Set(funcCall, "arguments", tc.Get("function.arguments").String())
							out, _ = sjson.SetRaw(out, "input.-1", funcCall)
						}
					}
				}
			}
		}
	}

	rf := gjson.GetBytes(rawJSON, "response_format")
	text := gjson.GetBytes(rawJSON, "text")
	if rf.Exists() {
		if !gjson.Get(out, "text").Exists() {
			out, _ = sjson.SetRaw(out, "text", `{}`)
		}
		switch rf.Get("type").String() {
		case "text":
			out, _ = sjson.Set(out, "text.format.type", "text")
		case "json_schema":
			if js := rf.Get("json_schema"); js.Exists() {
				out, _ = sjson.Set(out, "text.format.type", "json_schema")
				if v := js.Get("name"); v.Exists() {
					out, _ = sjson.Set(out, "text.format.name", v.Value())
				}
				if v := js.Get("strict"); v.Exists() {
					out, _ = sjson.Set(out, "text.format.strict", v.Value())
				}
				if v := js.Get("schema"); v.Exists() {
					out, _ = sjson.SetRaw(out, "text.format.schema", v.Raw)
				}
			}
		}
		if text.Exists() {
			if v := text.Get("verbosity"); v.Exists() {
				out, _ = sjson.Set(out, "text.verbosity", v.Value())
			}
		}
	} else if text.Exists() {
		if v := text.Get("verbosity"); v.Exists() {
			if !gjson.Get(out, "text").Exists() {
				out, _ = sjson.SetRaw(out, "text", `{}`)
			}
			out, _ = sjson.Set(out, "text.verbosity", v.Value())
		}
	}

	if tools := gjson.GetBytes(rawJSON, "tools"); tools.IsArray() && len(tools.Array()) > 0 {
		out, _ = sjson.SetRaw(out, "tools", `[]`)
		for _, t := range tools.Array() {
			toolType := t.Get("type").String()
			if toolType != "" && toolType != "function" && t.IsObject() {
				out, _ = sjson.SetRaw(out, "tools.-1", t.Raw)
				continue
			}
			if toolType == "function" {
				item := `{}`
				item, _ = sjson.Set(item, "type", "function")
				if v := t.Get("function.name"); v.Exists() {
					name := v.String()
					if short, ok := originalToolNameMap[name]; ok {
						name = short
					} else {
						name = shortenNameIfNeeded(name)
					}
					item, _ = sjson.Set(item, "name", name)
				}
				if v := t.Get("function.description"); v.Exists() {
					item, _ = sjson.Set(item, "description", v.Value())
				}
				if v := t.Get("function.parameters"); v.Exists() {
					item, _ = sjson.SetRaw(item, "parameters", v.Raw)
				}
				if v := t.Get("function.strict"); v.Exists() {
					item, _ = sjson.Set(item, "strict", v.Value())
				}
				out, _ = sjson.SetRaw(out, "tools.-1", item)
			}
		}
	}

	if tc := gjson.GetBytes(rawJSON, "tool_choice"); tc.Exists() {
		switch {
		case tc.Type == gjson.String:
			out, _ = sjson.Set(out, "tool_choice", tc.String())
		case tc.IsObject():
			tcType := tc.Get("type").String()
			if tcType == "function" {
				name := tc.Get("function.name").String()
				if name != "" {
					if short, ok := originalToolNameMap[name]; ok {
						name = short
					} else {
						name = shortenNameIfNeeded(name)
					}
				}
				choice := `{}`
				choice, _ = sjson.Set(choice, "type", "function")
				if name != "" {
					choice, _ = sjson.Set(choice, "name", name)
				}
				out, _ = sjson.SetRaw(out, "tool_choice", choice)
			} else if tcType != "" {
				out, _ = sjson.SetRaw(out, "tool_choice", tc.Raw)
			}
		}
	}

	out, _ = sjson.Set(out, "store", false)

	return []byte(out), nil
}

func (c *openaiToCodexResponse) Transform(body []byte) ([]byte, error) {
	return c.TransformWithState(body, nil)
}

func (c *openaiToCodexResponse) TransformWithState(body []byte, state *TransformState) ([]byte, error) {
	var tmp interface{}
	if err := json.Unmarshal(body, &tmp); err != nil {
		return nil, err
	}
	root := gjson.ParseBytes(body)
	requestRaw := []byte(nil)
	if state != nil {
		requestRaw = state.OriginalRequestBody
	}

	resp := `{"id":"","object":"response","created_at":0,"status":"completed","background":false,"error":null,"incomplete_details":null}`
	respID := root.Get("id").String()
	if respID == "" {
		respID = synthesizeResponseID()
	}
	resp, _ = sjson.Set(resp, "id", respID)

	created := root.Get("created").Int()
	if created == 0 {
		created = time.Now().Unix()
	}
	resp, _ = sjson.Set(resp, "created_at", created)

	if v := root.Get("model"); v.Exists() {
		resp, _ = sjson.Set(resp, "model", v.String())
	}

	outputsWrapper := `{"arr":[]}`

	if choices := root.Get("choices"); choices.Exists() && choices.IsArray() {
		choices.ForEach(func(_, choice gjson.Result) bool {
			msg := choice.Get("message")
			if msg.Exists() {
				if rc := msg.Get("reasoning_content"); rc.Exists() && rc.String() != "" {
					choiceIdx := int(choice.Get("index").Int())
					reasoning := `{"id":"","type":"reasoning","encrypted_content":"","summary":[]}`
					reasoning, _ = sjson.Set(reasoning, "id", fmt.Sprintf("rs_%s_%d", respID, choiceIdx))
					reasoning, _ = sjson.Set(reasoning, "summary.0.type", "summary_text")
					reasoning, _ = sjson.Set(reasoning, "summary.0.text", rc.String())
					outputsWrapper, _ = sjson.SetRaw(outputsWrapper, "arr.-1", reasoning)
				}
				if c := msg.Get("content"); c.Exists() && c.String() != "" {
					item := `{"id":"","type":"message","status":"completed","content":[{"type":"output_text","annotations":[],"logprobs":[],"text":""}],"role":"assistant"}`
					item, _ = sjson.Set(item, "id", fmt.Sprintf("msg_%s_%d", respID, int(choice.Get("index").Int())))
					item, _ = sjson.Set(item, "content.0.text", c.String())
					outputsWrapper, _ = sjson.SetRaw(outputsWrapper, "arr.-1", item)
				}

				if tcs := msg.Get("tool_calls"); tcs.Exists() && tcs.IsArray() {
					tcs.ForEach(func(_, tc gjson.Result) bool {
						callID := tc.Get("id").String()
						name := tc.Get("function.name").String()
						args := tc.Get("function.arguments").String()
						item := `{"id":"","type":"function_call","status":"completed","arguments":"","call_id":"","name":""}`
						item, _ = sjson.Set(item, "id", fmt.Sprintf("fc_%s", callID))
						item, _ = sjson.Set(item, "arguments", args)
						item, _ = sjson.Set(item, "call_id", callID)
						item, _ = sjson.Set(item, "name", name)
						outputsWrapper, _ = sjson.SetRaw(outputsWrapper, "arr.-1", item)
						return true
					})
				}
			}
			return true
		})
	}
	if gjson.Get(outputsWrapper, "arr.#").Int() > 0 {
		resp, _ = sjson.SetRaw(resp, "output", gjson.Get(outputsWrapper, "arr").Raw)
	}

	if usage := root.Get("usage"); usage.Exists() {
		if usage.Get("prompt_tokens").Exists() || usage.Get("completion_tokens").Exists() || usage.Get("total_tokens").Exists() {
			resp, _ = sjson.Set(resp, "usage.input_tokens", usage.Get("prompt_tokens").Int())
			if d := usage.Get("prompt_tokens_details.cached_tokens"); d.Exists() {
				resp, _ = sjson.Set(resp, "usage.input_tokens_details.cached_tokens", d.Int())
			}
			resp, _ = sjson.Set(resp, "usage.output_tokens", usage.Get("completion_tokens").Int())
			if d := usage.Get("completion_tokens_details.reasoning_tokens"); d.Exists() {
				resp, _ = sjson.Set(resp, "usage.output_tokens_details.reasoning_tokens", d.Int())
			} else if d := usage.Get("output_tokens_details.reasoning_tokens"); d.Exists() {
				resp, _ = sjson.Set(resp, "usage.output_tokens_details.reasoning_tokens", d.Int())
			}
			resp, _ = sjson.Set(resp, "usage.total_tokens", usage.Get("total_tokens").Int())
		} else {
			resp, _ = sjson.Set(resp, "usage", usage.Value())
		}
	}

	if len(requestRaw) > 0 {
		resp = applyRequestEchoToResponse(resp, "", requestRaw)
	}
	return []byte(resp), nil
}

func (c *openaiToCodexResponse) TransformChunk(chunk []byte, state *TransformState) ([]byte, error) {
	if state == nil {
		return nil, fmt.Errorf("TransformChunk requires non-nil state")
	}
	events, remaining := ParseSSE(state.Buffer + string(chunk))
	state.Buffer = remaining

	var output []byte
	for _, event := range events {
		if event.Event == "done" {
			continue
		}
		for _, item := range convertOpenAIChatCompletionsChunkToResponses(event.Data, state) {
			output = append(output, item...)
		}
	}

	return output, nil
}

type openaiToResponsesStateReasoning struct {
	ReasoningID   string
	ReasoningData string
}

type openaiToResponsesState struct {
	Seq              int
	ResponseID       string
	Created          int64
	Started          bool
	ReasoningID      string
	ReasoningIndex   int
	MsgTextBuf       map[int]*strings.Builder
	ReasoningBuf     strings.Builder
	Reasonings       []openaiToResponsesStateReasoning
	FuncArgsBuf      map[int]*strings.Builder
	FuncNames        map[int]string
	FuncCallIDs      map[int]string
	MsgItemAdded     map[int]bool
	MsgContentAdded  map[int]bool
	MsgItemDone      map[int]bool
	FuncArgsDone     map[int]bool
	FuncItemDone     map[int]bool
	PromptTokens     int64
	CachedTokens     int64
	CompletionTokens int64
	TotalTokens      int64
	ReasoningTokens  int64
	UsageSeen        bool
	NextOutputIndex  int            // global counter for unique output_index across messages and function calls
	MsgOutputIndex   map[int]int    // choice idx -> assigned output_index
	FuncOutputIndex  map[int]int    // callIndex -> assigned output_index
	CompletedSent    bool           // guards against duplicate response.completed
}

var responseIDCounter uint64

func synthesizeResponseID() string {
	return fmt.Sprintf("resp_%x_%d", time.Now().UnixNano(), atomic.AddUint64(&responseIDCounter, 1))
}

func (st *openaiToResponsesState) msgOutIdx(choiceIdx int) int {
	if oi, ok := st.MsgOutputIndex[choiceIdx]; ok {
		return oi
	}
	oi := st.NextOutputIndex
	st.MsgOutputIndex[choiceIdx] = oi
	st.NextOutputIndex++
	return oi
}

func (st *openaiToResponsesState) funcOutIdx(callIndex int) int {
	if oi, ok := st.FuncOutputIndex[callIndex]; ok {
		return oi
	}
	oi := st.NextOutputIndex
	st.FuncOutputIndex[callIndex] = oi
	st.NextOutputIndex++
	return oi
}

func convertOpenAIChatCompletionsChunkToResponses(rawJSON []byte, state *TransformState) [][]byte {
	if state == nil {
		return nil
	}
	st, ok := state.Custom.(*openaiToResponsesState)
	if !ok || st == nil {
		st = &openaiToResponsesState{
			FuncArgsBuf:     make(map[int]*strings.Builder),
			FuncNames:       make(map[int]string),
			FuncCallIDs:     make(map[int]string),
			MsgTextBuf:      make(map[int]*strings.Builder),
			MsgItemAdded:    make(map[int]bool),
			MsgContentAdded: make(map[int]bool),
			MsgItemDone:     make(map[int]bool),
			FuncArgsDone:    make(map[int]bool),
			FuncItemDone:    make(map[int]bool),
			Reasonings:      make([]openaiToResponsesStateReasoning, 0),
			MsgOutputIndex:  make(map[int]int),
			FuncOutputIndex: make(map[int]int),
		}
		state.Custom = st
	}

	root := gjson.ParseBytes(rawJSON)
	obj := root.Get("object")
	if obj.Exists() && obj.String() != "" && obj.String() != "chat.completion.chunk" {
		return nil
	}
	if !root.Get("choices").Exists() || !root.Get("choices").IsArray() {
		return nil
	}

	nextSeq := func() int { st.Seq++; return st.Seq }
	var out [][]byte

	if !st.Started {
		st.ResponseID = root.Get("id").String()
		if st.ResponseID == "" {
			st.ResponseID = synthesizeResponseID()
		}
		st.Created = root.Get("created").Int()
		if st.Created == 0 {
			st.Created = time.Now().Unix()
		}
		st.MsgTextBuf = make(map[int]*strings.Builder)
		st.ReasoningBuf.Reset()
		st.ReasoningID = ""
		st.ReasoningIndex = 0
		st.FuncArgsBuf = make(map[int]*strings.Builder)
		st.FuncNames = make(map[int]string)
		st.FuncCallIDs = make(map[int]string)
		st.MsgItemAdded = make(map[int]bool)
		st.MsgContentAdded = make(map[int]bool)
		st.MsgItemDone = make(map[int]bool)
		st.FuncArgsDone = make(map[int]bool)
		st.FuncItemDone = make(map[int]bool)
		st.MsgOutputIndex = make(map[int]int)
		st.FuncOutputIndex = make(map[int]int)
		st.NextOutputIndex = 0
		st.CompletedSent = false
		st.PromptTokens = 0
		st.CachedTokens = 0
		st.CompletionTokens = 0
		st.TotalTokens = 0
		st.ReasoningTokens = 0
		st.UsageSeen = false

		created := `{"type":"response.created","sequence_number":0,"response":{"id":"","object":"response","created_at":0,"status":"in_progress","background":false,"error":null,"output":[]}}`
		created, _ = sjson.Set(created, "sequence_number", nextSeq())
		created, _ = sjson.Set(created, "response.id", st.ResponseID)
		created, _ = sjson.Set(created, "response.created_at", st.Created)
		out = append(out, FormatSSE("response.created", []byte(created)))

		inprog := `{"type":"response.in_progress","sequence_number":0,"response":{"id":"","object":"response","created_at":0,"status":"in_progress"}}`
		inprog, _ = sjson.Set(inprog, "sequence_number", nextSeq())
		inprog, _ = sjson.Set(inprog, "response.id", st.ResponseID)
		inprog, _ = sjson.Set(inprog, "response.created_at", st.Created)
		out = append(out, FormatSSE("response.in_progress", []byte(inprog)))
		st.Started = true
	}

	if usage := root.Get("usage"); usage.Exists() {
		if v := usage.Get("prompt_tokens"); v.Exists() {
			st.PromptTokens = v.Int()
			st.UsageSeen = true
		}
		if v := usage.Get("prompt_tokens_details.cached_tokens"); v.Exists() {
			st.CachedTokens = v.Int()
			st.UsageSeen = true
		}
		if v := usage.Get("completion_tokens"); v.Exists() {
			st.CompletionTokens = v.Int()
			st.UsageSeen = true
		} else if v := usage.Get("output_tokens"); v.Exists() {
			st.CompletionTokens = v.Int()
			st.UsageSeen = true
		}
		if v := usage.Get("output_tokens_details.reasoning_tokens"); v.Exists() {
			st.ReasoningTokens = v.Int()
			st.UsageSeen = true
		} else if v := usage.Get("completion_tokens_details.reasoning_tokens"); v.Exists() {
			st.ReasoningTokens = v.Int()
			st.UsageSeen = true
		}
		if v := usage.Get("total_tokens"); v.Exists() {
			st.TotalTokens = v.Int()
			st.UsageSeen = true
		}
	}

	stopReasoning := func(text string) {
		textDone := `{"type":"response.reasoning_summary_text.done","sequence_number":0,"item_id":"","output_index":0,"summary_index":0,"text":""}`
		textDone, _ = sjson.Set(textDone, "sequence_number", nextSeq())
		textDone, _ = sjson.Set(textDone, "item_id", st.ReasoningID)
		textDone, _ = sjson.Set(textDone, "output_index", st.ReasoningIndex)
		textDone, _ = sjson.Set(textDone, "text", text)
		out = append(out, FormatSSE("response.reasoning_summary_text.done", []byte(textDone)))

		partDone := `{"type":"response.reasoning_summary_part.done","sequence_number":0,"item_id":"","output_index":0,"summary_index":0,"part":{"type":"summary_text","text":""}}`
		partDone, _ = sjson.Set(partDone, "sequence_number", nextSeq())
		partDone, _ = sjson.Set(partDone, "item_id", st.ReasoningID)
		partDone, _ = sjson.Set(partDone, "output_index", st.ReasoningIndex)
		partDone, _ = sjson.Set(partDone, "part.text", text)
		out = append(out, FormatSSE("response.reasoning_summary_part.done", []byte(partDone)))

		outputItemDone := `{"type":"response.output_item.done","item":{"id":"","type":"reasoning","encrypted_content":"","summary":[{"type":"summary_text","text":""}]},"output_index":0,"sequence_number":0}`
		outputItemDone, _ = sjson.Set(outputItemDone, "sequence_number", nextSeq())
		outputItemDone, _ = sjson.Set(outputItemDone, "item.id", st.ReasoningID)
		outputItemDone, _ = sjson.Set(outputItemDone, "output_index", st.ReasoningIndex)
		outputItemDone, _ = sjson.Set(outputItemDone, "item.summary.0.text", text)
		out = append(out, FormatSSE("response.output_item.done", []byte(outputItemDone)))

		st.Reasonings = append(st.Reasonings, openaiToResponsesStateReasoning{ReasoningID: st.ReasoningID, ReasoningData: text})
		st.ReasoningID = ""
	}

	if choices := root.Get("choices"); choices.Exists() && choices.IsArray() {
		choices.ForEach(func(_, choice gjson.Result) bool {
			idx := int(choice.Get("index").Int())
			delta := choice.Get("delta")
			if delta.Exists() {
				if c := delta.Get("content"); c.Exists() && c.String() != "" {
					if st.ReasoningID != "" {
						stopReasoning(st.ReasoningBuf.String())
						st.ReasoningBuf.Reset()
					}
					if !st.MsgItemAdded[idx] {
						item := `{"type":"response.output_item.added","sequence_number":0,"output_index":0,"item":{"id":"","type":"message","status":"in_progress","content":[],"role":"assistant"}}`
						item, _ = sjson.Set(item, "sequence_number", nextSeq())
						item, _ = sjson.Set(item, "output_index", st.msgOutIdx(idx))
						item, _ = sjson.Set(item, "item.id", fmt.Sprintf("msg_%s_%d", st.ResponseID, idx))
						out = append(out, FormatSSE("response.output_item.added", []byte(item)))
						st.MsgItemAdded[idx] = true
					}
					if !st.MsgContentAdded[idx] {
						part := `{"type":"response.content_part.added","sequence_number":0,"item_id":"","output_index":0,"content_index":0,"part":{"type":"output_text","annotations":[],"logprobs":[],"text":""}}`
						part, _ = sjson.Set(part, "sequence_number", nextSeq())
						part, _ = sjson.Set(part, "item_id", fmt.Sprintf("msg_%s_%d", st.ResponseID, idx))
						part, _ = sjson.Set(part, "output_index", st.msgOutIdx(idx))
						part, _ = sjson.Set(part, "content_index", 0)
						out = append(out, FormatSSE("response.content_part.added", []byte(part)))
						st.MsgContentAdded[idx] = true
					}

					msg := `{"type":"response.output_text.delta","sequence_number":0,"item_id":"","output_index":0,"content_index":0,"delta":"","logprobs":[]}`
					msg, _ = sjson.Set(msg, "sequence_number", nextSeq())
					msg, _ = sjson.Set(msg, "item_id", fmt.Sprintf("msg_%s_%d", st.ResponseID, idx))
					msg, _ = sjson.Set(msg, "output_index", st.msgOutIdx(idx))
					msg, _ = sjson.Set(msg, "content_index", 0)
					msg, _ = sjson.Set(msg, "delta", c.String())
					out = append(out, FormatSSE("response.output_text.delta", []byte(msg)))
					if st.MsgTextBuf[idx] == nil {
						st.MsgTextBuf[idx] = &strings.Builder{}
					}
					st.MsgTextBuf[idx].WriteString(c.String())
				}

				if rc := delta.Get("reasoning_content"); rc.Exists() && rc.String() != "" {
					if st.ReasoningID == "" {
						st.ReasoningID = fmt.Sprintf("rs_%s_%d", st.ResponseID, idx)
						st.ReasoningIndex = st.NextOutputIndex
					st.NextOutputIndex++
						item := `{"type":"response.output_item.added","sequence_number":0,"output_index":0,"item":{"id":"","type":"reasoning","status":"in_progress","summary":[]}}`
						item, _ = sjson.Set(item, "sequence_number", nextSeq())
						item, _ = sjson.Set(item, "output_index", st.ReasoningIndex)
						item, _ = sjson.Set(item, "item.id", st.ReasoningID)
						out = append(out, FormatSSE("response.output_item.added", []byte(item)))
						part := `{"type":"response.reasoning_summary_part.added","sequence_number":0,"item_id":"","output_index":0,"summary_index":0,"part":{"type":"summary_text","text":""}}`
						part, _ = sjson.Set(part, "sequence_number", nextSeq())
						part, _ = sjson.Set(part, "item_id", st.ReasoningID)
						part, _ = sjson.Set(part, "output_index", st.ReasoningIndex)
						out = append(out, FormatSSE("response.reasoning_summary_part.added", []byte(part)))
					}
					st.ReasoningBuf.WriteString(rc.String())
					msg := `{"type":"response.reasoning_summary_text.delta","sequence_number":0,"item_id":"","output_index":0,"summary_index":0,"delta":""}`
					msg, _ = sjson.Set(msg, "sequence_number", nextSeq())
					msg, _ = sjson.Set(msg, "item_id", st.ReasoningID)
					msg, _ = sjson.Set(msg, "output_index", st.ReasoningIndex)
					msg, _ = sjson.Set(msg, "delta", rc.String())
					out = append(out, FormatSSE("response.reasoning_summary_text.delta", []byte(msg)))
				}

				if tcs := delta.Get("tool_calls"); tcs.Exists() && tcs.IsArray() {
					if st.ReasoningID != "" {
						stopReasoning(st.ReasoningBuf.String())
						st.ReasoningBuf.Reset()
					}
					if st.MsgItemAdded[idx] && !st.MsgItemDone[idx] {
						fullText := ""
						if b := st.MsgTextBuf[idx]; b != nil {
							fullText = b.String()
						}
						done := `{"type":"response.output_text.done","sequence_number":0,"item_id":"","output_index":0,"content_index":0,"text":"","logprobs":[]}`
						done, _ = sjson.Set(done, "sequence_number", nextSeq())
						done, _ = sjson.Set(done, "item_id", fmt.Sprintf("msg_%s_%d", st.ResponseID, idx))
						done, _ = sjson.Set(done, "output_index", st.msgOutIdx(idx))
						done, _ = sjson.Set(done, "content_index", 0)
						done, _ = sjson.Set(done, "text", fullText)
						out = append(out, FormatSSE("response.output_text.done", []byte(done)))

						partDone := `{"type":"response.content_part.done","sequence_number":0,"item_id":"","output_index":0,"content_index":0,"part":{"type":"output_text","annotations":[],"logprobs":[],"text":""}}`
						partDone, _ = sjson.Set(partDone, "sequence_number", nextSeq())
						partDone, _ = sjson.Set(partDone, "item_id", fmt.Sprintf("msg_%s_%d", st.ResponseID, idx))
						partDone, _ = sjson.Set(partDone, "output_index", st.msgOutIdx(idx))
						partDone, _ = sjson.Set(partDone, "content_index", 0)
						partDone, _ = sjson.Set(partDone, "part.text", fullText)
						out = append(out, FormatSSE("response.content_part.done", []byte(partDone)))

						itemDone := `{"type":"response.output_item.done","sequence_number":0,"output_index":0,"item":{"id":"","type":"message","status":"completed","content":[{"type":"output_text","annotations":[],"logprobs":[],"text":""}],"role":"assistant"}}`
						itemDone, _ = sjson.Set(itemDone, "sequence_number", nextSeq())
						itemDone, _ = sjson.Set(itemDone, "output_index", st.msgOutIdx(idx))
						itemDone, _ = sjson.Set(itemDone, "item.id", fmt.Sprintf("msg_%s_%d", st.ResponseID, idx))
						itemDone, _ = sjson.Set(itemDone, "item.content.0.text", fullText)
						out = append(out, FormatSSE("response.output_item.done", []byte(itemDone)))
						st.MsgItemDone[idx] = true
					}

					for tcIndex, tc := range tcs.Array() {
						callIndex := tcIndex
						if v := tc.Get("index"); v.Exists() {
							callIndex = int(v.Int())
						}

						newCallID := tc.Get("id").String()
						nameChunk := tc.Get("function.name").String()
						if nameChunk != "" {
							st.FuncNames[callIndex] = nameChunk
						}
						existingCallID := st.FuncCallIDs[callIndex]
						effectiveCallID := existingCallID
						shouldEmitItem := false
						if existingCallID == "" && newCallID != "" {
							effectiveCallID = newCallID
							st.FuncCallIDs[callIndex] = newCallID
							shouldEmitItem = true
						}

						if shouldEmitItem && effectiveCallID != "" {
							o := `{"type":"response.output_item.added","sequence_number":0,"output_index":0,"item":{"id":"","type":"function_call","status":"in_progress","arguments":"","call_id":"","name":""}}`
							o, _ = sjson.Set(o, "sequence_number", nextSeq())
							o, _ = sjson.Set(o, "output_index", st.funcOutIdx(callIndex))
							o, _ = sjson.Set(o, "item.id", fmt.Sprintf("fc_%s", effectiveCallID))
							o, _ = sjson.Set(o, "item.call_id", effectiveCallID)
							o, _ = sjson.Set(o, "item.name", st.FuncNames[callIndex])
							out = append(out, FormatSSE("response.output_item.added", []byte(o)))
						}

						if st.FuncArgsBuf[callIndex] == nil {
							st.FuncArgsBuf[callIndex] = &strings.Builder{}
						}
						if args := tc.Get("function.arguments"); args.Exists() && args.String() != "" {
							refCallID := st.FuncCallIDs[callIndex]
							if refCallID == "" {
								refCallID = newCallID
							}
							if refCallID != "" {
								ad := `{"type":"response.function_call_arguments.delta","sequence_number":0,"item_id":"","output_index":0,"delta":""}`
								ad, _ = sjson.Set(ad, "sequence_number", nextSeq())
								ad, _ = sjson.Set(ad, "item_id", fmt.Sprintf("fc_%s", refCallID))
								ad, _ = sjson.Set(ad, "output_index", st.funcOutIdx(callIndex))
								ad, _ = sjson.Set(ad, "delta", args.String())
								out = append(out, FormatSSE("response.function_call_arguments.delta", []byte(ad)))
							}
							st.FuncArgsBuf[callIndex].WriteString(args.String())
						}
					}
				}
			}

			if fr := choice.Get("finish_reason"); fr.Exists() && fr.String() != "" {
				if len(st.MsgItemAdded) > 0 {
					for _, i := range sortedKeys(st.MsgItemAdded) {
						if st.MsgItemAdded[i] && !st.MsgItemDone[i] {
							fullText := ""
							if b := st.MsgTextBuf[i]; b != nil {
								fullText = b.String()
							}
							done := `{"type":"response.output_text.done","sequence_number":0,"item_id":"","output_index":0,"content_index":0,"text":"","logprobs":[]}`
							done, _ = sjson.Set(done, "sequence_number", nextSeq())
							done, _ = sjson.Set(done, "item_id", fmt.Sprintf("msg_%s_%d", st.ResponseID, i))
							done, _ = sjson.Set(done, "output_index", st.msgOutIdx(i))
							done, _ = sjson.Set(done, "content_index", 0)
							done, _ = sjson.Set(done, "text", fullText)
							out = append(out, FormatSSE("response.output_text.done", []byte(done)))

							partDone := `{"type":"response.content_part.done","sequence_number":0,"item_id":"","output_index":0,"content_index":0,"part":{"type":"output_text","annotations":[],"logprobs":[],"text":""}}`
							partDone, _ = sjson.Set(partDone, "sequence_number", nextSeq())
							partDone, _ = sjson.Set(partDone, "item_id", fmt.Sprintf("msg_%s_%d", st.ResponseID, i))
							partDone, _ = sjson.Set(partDone, "output_index", st.msgOutIdx(i))
							partDone, _ = sjson.Set(partDone, "content_index", 0)
							partDone, _ = sjson.Set(partDone, "part.text", fullText)
							out = append(out, FormatSSE("response.content_part.done", []byte(partDone)))

							itemDone := `{"type":"response.output_item.done","sequence_number":0,"output_index":0,"item":{"id":"","type":"message","status":"completed","content":[{"type":"output_text","annotations":[],"logprobs":[],"text":""}],"role":"assistant"}}`
							itemDone, _ = sjson.Set(itemDone, "sequence_number", nextSeq())
							itemDone, _ = sjson.Set(itemDone, "output_index", st.msgOutIdx(i))
							itemDone, _ = sjson.Set(itemDone, "item.id", fmt.Sprintf("msg_%s_%d", st.ResponseID, i))
							itemDone, _ = sjson.Set(itemDone, "item.content.0.text", fullText)
							out = append(out, FormatSSE("response.output_item.done", []byte(itemDone)))
							st.MsgItemDone[i] = true
						}
					}
				}

				if st.ReasoningID != "" {
					stopReasoning(st.ReasoningBuf.String())
					st.ReasoningBuf.Reset()
				}

				if len(st.FuncCallIDs) > 0 {
					for _, i := range sortedKeys(st.FuncCallIDs) {
						callID := st.FuncCallIDs[i]
						if callID == "" || st.FuncItemDone[i] {
							continue
						}
						args := "{}"
						if b := st.FuncArgsBuf[i]; b != nil && b.Len() > 0 {
							args = b.String()
						}
						fcDone := `{"type":"response.function_call_arguments.done","sequence_number":0,"item_id":"","output_index":0,"arguments":""}`
						fcDone, _ = sjson.Set(fcDone, "sequence_number", nextSeq())
						fcDone, _ = sjson.Set(fcDone, "item_id", fmt.Sprintf("fc_%s", callID))
						fcDone, _ = sjson.Set(fcDone, "output_index", st.funcOutIdx(i))
						fcDone, _ = sjson.Set(fcDone, "arguments", args)
						out = append(out, FormatSSE("response.function_call_arguments.done", []byte(fcDone)))

						itemDone := `{"type":"response.output_item.done","sequence_number":0,"output_index":0,"item":{"id":"","type":"function_call","status":"completed","arguments":"","call_id":"","name":""}}`
						itemDone, _ = sjson.Set(itemDone, "sequence_number", nextSeq())
						itemDone, _ = sjson.Set(itemDone, "output_index", st.funcOutIdx(i))
						itemDone, _ = sjson.Set(itemDone, "item.id", fmt.Sprintf("fc_%s", callID))
						itemDone, _ = sjson.Set(itemDone, "item.arguments", args)
						itemDone, _ = sjson.Set(itemDone, "item.call_id", callID)
						itemDone, _ = sjson.Set(itemDone, "item.name", st.FuncNames[i])
						out = append(out, FormatSSE("response.output_item.done", []byte(itemDone)))
						st.FuncItemDone[i] = true
						st.FuncArgsDone[i] = true
					}
				}
			}
			return true
		})
	}

	// Emit response.completed once after all choices have been processed
	if !st.CompletedSent {
		// Check if any choice had a finish_reason
		hasFinish := false
		if choices := root.Get("choices"); choices.Exists() && choices.IsArray() {
			choices.ForEach(func(_, choice gjson.Result) bool {
				if fr := choice.Get("finish_reason"); fr.Exists() && fr.String() != "" {
					hasFinish = true
					return false
				}
				return true
			})
		}
		if hasFinish {
			st.CompletedSent = true
			completed := `{"type":"response.completed","sequence_number":0,"response":{"id":"","object":"response","created_at":0,"status":"completed","background":false,"error":null}}`
			completed, _ = sjson.Set(completed, "sequence_number", nextSeq())
			completed, _ = sjson.Set(completed, "response.id", st.ResponseID)
			completed, _ = sjson.Set(completed, "response.created_at", st.Created)

			outputsWrapper := `{"arr":[]}`
			if len(st.Reasonings) > 0 {
				for _, r := range st.Reasonings {
					item := `{"id":"","type":"reasoning","summary":[{"type":"summary_text","text":""}]}`
					item, _ = sjson.Set(item, "id", r.ReasoningID)
					item, _ = sjson.Set(item, "summary.0.text", r.ReasoningData)
					outputsWrapper, _ = sjson.SetRaw(outputsWrapper, "arr.-1", item)
				}
			}
			if len(st.MsgItemAdded) > 0 {
				for _, i := range sortedKeys(st.MsgItemAdded) {
					txt := ""
					if b := st.MsgTextBuf[i]; b != nil {
						txt = b.String()
					}
					item := `{"id":"","type":"message","status":"completed","content":[{"type":"output_text","annotations":[],"logprobs":[],"text":""}],"role":"assistant"}`
					item, _ = sjson.Set(item, "id", fmt.Sprintf("msg_%s_%d", st.ResponseID, i))
					item, _ = sjson.Set(item, "content.0.text", txt)
					outputsWrapper, _ = sjson.SetRaw(outputsWrapper, "arr.-1", item)
				}
			}
			if len(st.FuncCallIDs) > 0 {
				for _, i := range sortedKeys(st.FuncCallIDs) {
					args := ""
					if b := st.FuncArgsBuf[i]; b != nil {
						args = b.String()
					}
					callID := st.FuncCallIDs[i]
					name := st.FuncNames[i]
					item := `{"id":"","type":"function_call","status":"completed","arguments":"","call_id":"","name":""}`
					item, _ = sjson.Set(item, "id", fmt.Sprintf("fc_%s", callID))
					item, _ = sjson.Set(item, "arguments", args)
					item, _ = sjson.Set(item, "call_id", callID)
					item, _ = sjson.Set(item, "name", name)
					outputsWrapper, _ = sjson.SetRaw(outputsWrapper, "arr.-1", item)
				}
			}
			if gjson.Get(outputsWrapper, "arr.#").Int() > 0 {
				completed, _ = sjson.SetRaw(completed, "response.output", gjson.Get(outputsWrapper, "arr").Raw)
			}
			if st.UsageSeen {
				completed, _ = sjson.Set(completed, "response.usage.input_tokens", st.PromptTokens)
				completed, _ = sjson.Set(completed, "response.usage.input_tokens_details.cached_tokens", st.CachedTokens)
				completed, _ = sjson.Set(completed, "response.usage.output_tokens", st.CompletionTokens)
				if st.ReasoningTokens > 0 {
					completed, _ = sjson.Set(completed, "response.usage.output_tokens_details.reasoning_tokens", st.ReasoningTokens)
				}
				total := st.TotalTokens
				if total == 0 {
					total = st.PromptTokens + st.CompletionTokens
				}
				completed, _ = sjson.Set(completed, "response.usage.total_tokens", total)
			}
			if len(state.OriginalRequestBody) > 0 {
				completed = applyRequestEchoToResponse(completed, "response.", state.OriginalRequestBody)
			}
			out = append(out, FormatSSE("response.completed", []byte(completed)))
		}
	}

	return out
}

func sortedKeys[T any](m map[int]T) []int {
	keys := make([]int, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	return keys
}

func applyRequestEchoToResponse(responseJSON string, prefix string, requestRaw []byte) string {
	if len(requestRaw) == 0 {
		return responseJSON
	}
	req := gjson.ParseBytes(requestRaw)
	paths := []string{
		"model",
		"instructions",
		"input",
		"tools",
		"tool_choice",
		"metadata",
		"store",
		"max_output_tokens",
		"temperature",
		"top_p",
		"reasoning",
		"parallel_tool_calls",
		"include",
		"previous_response_id",
		"text",
		"truncation",
		"service_tier",
	}
	for _, path := range paths {
		val := req.Get(path)
		if !val.Exists() {
			continue
		}
		fullPath := prefix + path
		if gjson.Get(responseJSON, fullPath).Exists() {
			continue
		}
		switch val.Type {
		case gjson.String, gjson.Number, gjson.True, gjson.False:
			responseJSON, _ = sjson.Set(responseJSON, fullPath, val.Value())
		default:
			responseJSON, _ = sjson.SetRaw(responseJSON, fullPath, val.Raw)
		}
	}
	return responseJSON
}

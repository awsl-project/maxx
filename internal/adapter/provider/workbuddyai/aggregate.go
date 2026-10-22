package workbuddyai

import (
	"encoding/json"
	"sort"
)

// Retain only assembled output for non-streaming clients, with a hard memory cap.
const maxAggregateBytes = 32 << 20

type chatAggregate struct {
	meta    map[string]json.RawMessage
	choices map[int]*aggregateChoice
	size    int
}
type aggregateChoice struct {
	message  map[string]interface{}
	tools    map[int]map[string]interface{}
	finish   json.RawMessage
	logprobs json.RawMessage
}

func newChatAggregate() *chatAggregate {
	return &chatAggregate{meta: map[string]json.RawMessage{}, choices: map[int]*aggregateChoice{}}
}
func (a *chatAggregate) add(frame map[string]json.RawMessage) error {
	for _, k := range []string{"id", "created", "model", "system_fingerprint", "service_tier", "usage"} {
		if v, ok := frame[k]; ok {
			a.meta[k] = v
		}
	}
	var choices []struct {
		Index    int                    `json:"index"`
		Delta    map[string]interface{} `json:"delta"`
		Finish   json.RawMessage        `json:"finish_reason"`
		Logprobs json.RawMessage        `json:"logprobs"`
	}
	if err := json.Unmarshal(frame["choices"], &choices); err != nil {
		return upstreamError("invalid WorkBuddy chat choices", 502)
	}
	for _, ch := range choices {
		c := a.choices[ch.Index]
		if c == nil {
			c = &aggregateChoice{message: map[string]interface{}{"role": "assistant", "content": nil}, tools: map[int]map[string]interface{}{}}
			a.choices[ch.Index] = c
		}
		for k, v := range ch.Delta {
			if k == "function_call" {
				fn, _ := v.(map[string]interface{})
				dst, _ := c.message[k].(map[string]interface{})
				if dst == nil {
					dst = map[string]interface{}{}
					c.message[k] = dst
				}
				for fk, fv := range fn {
					if str, ok := fv.(string); ok {
						old, _ := dst[fk].(string)
						dst[fk] = old + str
						a.size += len(str)
					} else {
						dst[fk] = fv
					}
				}
				continue
			}
			if k == "tool_calls" {
				list, _ := v.([]interface{})
				for _, raw := range list {
					tool, ok := raw.(map[string]interface{})
					if !ok {
						continue
					}
					index, _ := tool["index"].(float64)
					i := int(index)
					t := c.tools[i]
					if t == nil {
						t = map[string]interface{}{}
						c.tools[i] = t
					}
					for tk, tv := range tool {
						if tk == "index" {
							continue
						}
						if tk == "function" {
							fn, _ := tv.(map[string]interface{})
							dst, _ := t["function"].(map[string]interface{})
							if dst == nil {
								dst = map[string]interface{}{}
								t["function"] = dst
							}
							for fk, fv := range fn {
								if s, ok := fv.(string); ok {
									old, _ := dst[fk].(string)
									dst[fk] = old + s
									a.size += len(s)
								} else {
									dst[fk] = fv
								}
							}
						} else {
							t[tk] = tv
						}
					}
				}
				continue
			}
			if s, ok := v.(string); ok && k != "role" {
				old, _ := c.message[k].(string)
				c.message[k] = old + s
				a.size += len(s)
			} else if v != nil {
				c.message[k] = v
			}
		}
		if len(ch.Finish) > 0 && string(ch.Finish) != "null" {
			c.finish = ch.Finish
		}
		if len(ch.Logprobs) > 0 && string(ch.Logprobs) != "null" {
			c.logprobs = ch.Logprobs
		}
	}
	if a.size > maxAggregateBytes {
		return upstreamError("WorkBuddy completion exceeds aggregation limit", 502)
	}
	return nil
}
func (a *chatAggregate) finish() ([]byte, error) {
	if len(a.choices) == 0 {
		return nil, upstreamError("WorkBuddy returned no completion", 502)
	}
	out := map[string]interface{}{"object": "chat.completion"}
	for k, v := range a.meta {
		out[k] = v
	}
	indices := make([]int, 0, len(a.choices))
	for i := range a.choices {
		indices = append(indices, i)
	}
	sort.Ints(indices)
	choices := make([]interface{}, 0, len(indices))
	for _, i := range indices {
		c := a.choices[i]
		if len(c.finish) == 0 {
			return nil, upstreamError("WorkBuddy completion has no finish reason", 502)
		}
		if len(c.tools) > 0 {
			ids := make([]int, 0, len(c.tools))
			for id := range c.tools {
				ids = append(ids, id)
			}
			sort.Ints(ids)
			tools := make([]interface{}, 0, len(ids))
			for _, id := range ids {
				tools = append(tools, c.tools[id])
			}
			c.message["tool_calls"] = tools
		}
		choices = append(choices, map[string]interface{}{"index": i, "message": c.message, "finish_reason": c.finish, "logprobs": c.logprobs})
	}
	out["choices"] = choices
	return json.Marshal(out)
}

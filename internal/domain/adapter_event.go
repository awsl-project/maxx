package domain

// AdapterEventType represents the type of adapter event
type AdapterEventType int

const (
	// EventRequestInfo is sent when upstream request is made
	EventRequestInfo AdapterEventType = iota
	// EventResponseInfo is sent when upstream response is received
	EventResponseInfo
	// EventMetrics is sent when token usage is extracted
	EventMetrics
	// EventResponseModel is sent when response model is extracted
	EventResponseModel
)

// AdapterMetrics contains token usage metrics (avoids import cycle with usage package)
type AdapterMetrics struct {
	InputTokens          uint64
	OutputTokens         uint64
	CacheReadCount       uint64
	CacheCreationCount   uint64
	Cache5mCreationCount uint64
	Cache1hCreationCount uint64
}

// AdapterEvent represents an event from adapter to executor
type AdapterEvent struct {
	Type          AdapterEventType
	RequestInfo   *RequestInfo    // for EventRequestInfo
	ResponseInfo  *ResponseInfo   // for EventResponseInfo
	Metrics       *AdapterMetrics // for EventMetrics
	ResponseModel string          // for EventResponseModel
}

// AdapterEventChan is used by adapters to send events to executor
type AdapterEventChan chan *AdapterEvent

// NewAdapterEventChan creates a buffered event channel
func NewAdapterEventChan() AdapterEventChan {
	return make(chan *AdapterEvent, 10)
}

// SendRequestInfo sends request info event
func (ch AdapterEventChan) SendRequestInfo(info *RequestInfo) {
	if ch == nil || info == nil {
		return
	}
	select {
	case ch <- &AdapterEvent{Type: EventRequestInfo, RequestInfo: info}:
	default:
		// Channel full, skip
	}
}

// SendResponseInfo sends response info event
func (ch AdapterEventChan) SendResponseInfo(info *ResponseInfo) {
	if ch == nil || info == nil {
		return
	}
	select {
	case ch <- &AdapterEvent{Type: EventResponseInfo, ResponseInfo: info}:
	default:
	}
}

// SendMetrics sends metrics event
func (ch AdapterEventChan) SendMetrics(metrics *AdapterMetrics) {
	if ch == nil || metrics == nil {
		return
	}
	select {
	case ch <- &AdapterEvent{Type: EventMetrics, Metrics: metrics}:
	default:
	}
}

// SendResponseModel sends response model event
func (ch AdapterEventChan) SendResponseModel(model string) {
	if ch == nil || model == "" {
		return
	}
	select {
	case ch <- &AdapterEvent{Type: EventResponseModel, ResponseModel: model}:
	default:
	}
}

// Close closes the event channel
func (ch AdapterEventChan) Close() {
	if ch != nil {
		close(ch)
	}
}

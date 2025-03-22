package schema

import "errors"

const (
	ScopeCluster = "cluster"
	ScopeNode    = "node"
)

const (
	EventTypeLog = "log"
	EventTypeK8s = "k8s"
)

const (
	FieldId                   = "id"
	FieldHash                 = "hash"
	FieldSeverity             = "severity"
	FieldGeneration           = "gen"
	FieldKind                 = "kind"
	FieldTags                 = "tags"
	FieldMessage              = "message"
	FieldName                 = "name"
	FieldDisplayName          = "displayName"
	FieldDescription          = "description"
	FieldImageUrl             = "image_url"
	FieldTimestamp            = "timestamp"
	FieldK8sEventReason       = "reason"
	FieldK8sEventType         = "type"
	FieldK8sEventReasonDetail = "reason_detail"
)

const (
	KindRules      = "rules"
	KindTags       = "tags"
	KindCategories = "categories"
)
const (
	PropFieldTimestamp = "timestamp"
	PropFieldSpoolIdx  = "spool_idx"
)

var (
	ErrField = errors.New("invalid field")
)

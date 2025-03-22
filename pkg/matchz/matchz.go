package matchz

import "errors"

var ErrNoMatches = errors.New("no matches")

//go:generate msgp
type MatchesT struct {
	Timestamp int64  `msg:"ts"`
	SpoolIdx  int64  `msg:"spidx"`
	RuleId    string `msg:"rid"`
	RuleHash  string `msg:"rhash"`
	MatchId   uint32 `msg:"matchid"`
	Depth     uint32 `msg:"depth"`
	NodeType  string `msg:"ntype"`
	Hits      HitsT  `msg:"hits"`
}

type HitsT struct {
	Count        uint32          `msg:"cnt"`
	Entries      []EntryT        `msg:"entries"`
	Correlations []CorrelationT  `msg:"corrs"`
	Entity       EntityMetadataT `msg:"entity"`
}

// Return first timestamp
func (h HitsT) Timestamp() (int64, error) {
	for _, e := range h.Entries {
		if e.Timestamp != 0 {
			return e.Timestamp, nil
		}
	}
	return 0, ErrNoMatches
}

type CorrelationT struct {
	Field    string `msg:"field"`
	StrValue string `msg:"strv"`
	IntValue int64  `msg:"intv"`
}

type EntryT struct {
	Timestamp int64  `msg:"ts"`
	Entry     []byte `msg:"entry"`
	SpoolIdx  int64  `msg:"spidx"`
}

type EntityMetadataT struct {
	ProcessId     uint32 `msg:"pid"`
	MachineId     string `msg:"mid"`
	CgroupId      string `msg:"cgrp"`
	ContainerId   string `msg:"cid"`
	PodName       string `msg:"pod"`
	HostName      string `msg:"host"`
	Namespace     string `msg:"ns"`
	FileName      string `msg:"fname"`
	ProcessName   string `msg:"pname"`
	ImageUrl      string `msg:"imgurl"`
	ContainerName string `msg:"cname"`
	Origin        bool   `msg:"orig"`
}

package matchz

type MatchesT struct {
	Hits HitsT
}

type HitsT struct {
	Count   uint32
	Entries []EntryT
	Entity  EntityMetadataT
}

type EntryT struct {
	Timestamp int64
	Entry     []byte
}

type EntityMetadataT struct {
	FileName string
	Origin   bool
}

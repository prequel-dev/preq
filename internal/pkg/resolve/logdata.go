package resolve

import "errors"

// LogData is a collection of log src
// It implements the DataSrc interface
type LogData struct {
	name    string
	srcType string
	Logs    []LogSrcI
}

func NewLogData(logs []LogSrcI, name, srcType string) *LogData {

	return &LogData{
		Logs:    logs,
		name:    name,
		srcType: srcType,
	}
}

func (ld *LogData) Name() string {
	return ld.name
}

func (ld *LogData) SrcType() string {
	return ld.srcType
}

func (ld *LogData) Type() string {
	return logType
}

func (ld *LogData) Meta() map[string]string {
	return nil
}

func (ld *LogData) Size() int64 {
	var total int64
	for _, log := range ld.Logs {
		sz := log.Size()
		if sz < 0 {
			// we are out of luck; one of the sources is a pipe or compressed
			return -1
		}
		total += sz
	}
	return total
}

func (ld *LogData) Close() error {
	var errList []error
	for _, log := range ld.Logs {
		if err := log.Close(); err != nil {
			errList = append(errList, err)
		}
	}
	ld.Logs = nil
	return errors.Join(errList...)
}

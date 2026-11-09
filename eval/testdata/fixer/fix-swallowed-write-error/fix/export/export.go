package export

import (
	"fmt"
	"io"
)

// Record is one row of the usage export.
type Record struct {
	Name  string
	Count int
}

// WriteCSV writes one name,count line per record and returns the first write error.
func WriteCSV(w io.Writer, records []Record) error {
	for _, r := range records {
		if _, err := fmt.Fprintf(w, "%s,%d\n", r.Name, r.Count); err != nil {
			return err
		}
	}
	return nil
}

package main

import (
	"bufio"
	"strings"
	"testing"
)

func TestLoadReports(t *testing.T) {
	scanner := bufio.NewScanner(strings.NewReader("1710000000,a,b,c\n1710003600,d,e\n"))

	reports, bulkReports, err := loadReports(scanner)
	if err != nil {
		t.Fatalf("loadReports returned error: %v", err)
	}

	values, ok := reports.Get("1710000000")
	if !ok {
		t.Fatalf("expected first report to be present")
	}

	if len(values) != 3 || values[0] != "a" || values[1] != "b" || values[2] != "c" {
		t.Fatalf("unexpected parsed values: %#v", values)
	}

	line, ok := bulkReports.Get("1710003600")
	if !ok {
		t.Fatalf("expected second bulk report to be present")
	}

	if line != "1710003600,d,e" {
		t.Fatalf("unexpected bulk report line: %q", line)
	}
}

func TestLoadReportsRejectsMalformedLine(t *testing.T) {
	scanner := bufio.NewScanner(strings.NewReader("not-a-valid-row\n"))

	_, _, err := loadReports(scanner)
	if err == nil {
		t.Fatal("expected malformed line to return an error")
	}
}

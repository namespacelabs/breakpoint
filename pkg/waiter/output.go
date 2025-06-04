package waiter

import (
	"fmt"
	"io"
	"net"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/muesli/reflow/wordwrap"
)

func PrintConnectionInfo(status ManagerStatus, output io.Writer) {
	host, port, _ := net.SplitHostPort(status.Endpoint)
	deadline := status.Expiration

	if host == "" && port == "" {
		return
	}

	ww := wordwrap.NewWriter(80)
	fmt.Fprintf(ww, "Breakpoint! Running until %v (%v).", deadline.Format(Stamp), humanize.Time(deadline))
	_ = ww.Close()

	lines := strings.Split(ww.String(), "\n")

	longestLine := 0
	for _, l := range lines {
		if len(l) > longestLine {
			longestLine = len(l)
		}
	}

	longline := nchars('─', longestLine)
	spaces := nchars(' ', longestLine)
	fmt.Fprintln(output)
	fmt.Fprintf(output, "┌─%s─┐\n", longline)
	for _, l := range lines {
		fmt.Fprintf(output, "│ %s%s │\n", l, spaces[len(l):])
	}
	fmt.Fprintf(output, "└─%s─┘\n", longline)
	fmt.Fprintln(output)

	fmt.Fprintf(output, "Connect with:\n\n")
	fmt.Fprintf(output, "ssh -p %s runner@%s\n", port, host)
}

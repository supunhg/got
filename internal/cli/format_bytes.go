package cli

import "fmt"

// formatBytes renders a byte count as a human-readable string.
// Used by `got inspect`'s statistics and language tables.
//
// The format mirrors `du -h` and GitHub:
//
//	0            → "0 B"
//	1023         → "1023 B"
//	1024         → "1.0 KiB"
//	1536         → "1.5 KiB"
//	1048576      → "1.0 MiB"
//	1073741824   → "1.0 GiB"
//	1099511627776 → "1.0 TiB"
//
// We always use KiB / MiB / GiB (binary) rather than KB / MB /
// GB (decimal) so the rendered number matches what users see
// in `ls -lh` and in their IDEs.
func formatBytes(n int64) string {
	const (
		kib = 1024
		mib = 1024 * kib
		gib = 1024 * mib
		tib = 1024 * gib
	)
	switch {
	case n < kib:
		return fmt.Sprintf("%d B", n)
	case n < mib:
		return fmt.Sprintf("%.1f KiB", float64(n)/float64(kib))
	case n < gib:
		return fmt.Sprintf("%.1f MiB", float64(n)/float64(mib))
	case n < tib:
		return fmt.Sprintf("%.1f GiB", float64(n)/float64(gib))
	}
	return fmt.Sprintf("%.1f TiB", float64(n)/float64(tib))
}

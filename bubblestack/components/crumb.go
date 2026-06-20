package components

// crumbSeg resolves a screen's breadcrumb segment: the short form when requested and
// set, else the explicit crumb, else the type's fallback/default. Shared by the
// components' CrumbLabel implementations so the "short → full → fallback" rule (and
// each type's default crumb) lives in one place.
func crumbSeg(short bool, crumbShort, crumb, fallback string) string {
	if short && crumbShort != "" {
		return crumbShort
	}
	if crumb != "" {
		return crumb
	}
	return fallback
}

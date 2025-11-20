package b2bclient

func SliceToMapByRef(items []EnabledOutputView) map[string]EnabledOutputView {
	out := make(map[string]EnabledOutputView, len(items))
	for _, v := range items {
		out[v.Ref] = v
	}
	return out
}

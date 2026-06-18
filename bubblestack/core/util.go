package core

func GetOptional[T any](def T, vars ...T) T {
	if len(vars) == 0 {
		return def
	}
	return vars[0]
}

func GetOptionalIdx[T any](def T, idx int, vars ...T) T {
	if len(vars)-1 < idx {
		return def
	}
	return vars[idx]
}

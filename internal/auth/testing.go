package auth

// SetFastParamsForTests baja el coste de argon2 (sin esto cada hash de los
// tests tarda ~100 ms y 64 MB). Solo para tests; devuelve la función que
// restaura los valores reales.
func SetFastParamsForTests() (restore func()) {
	t, m, p := argonTime, argonMemory, argonThreads
	argonTime, argonMemory, argonThreads = 1, 8, 1
	return func() { argonTime, argonMemory, argonThreads = t, m, p }
}

package auth

// fakeArgon2idHash — заранее посчитанный хеш несуществующего пароля.
// Используется в Login, когда email не найден: Verify выполняет полное вычисление argon2id
// вместо мгновенного возврата, чтобы latency не утекала информацию о существовании email.
// Перегенерировать командой: go run ./internal/pkg/password/cmd/genhash.go
const fakeArgon2idHash = "$argon2id$v=19$m=65536,t=2,p=2$ppcX/HW3oxihoKjK4jOKRQ$gWgGHpky9lPtec/qZ5S8EEQ4ilVubFcI38bh1wtvZ+I"

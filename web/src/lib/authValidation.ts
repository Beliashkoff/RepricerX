export const passwordRequirementText = 'Пароль должен быть от 12 до 128 символов, содержать букву и цифру'

export function isValidEmail(email: string) {
  return /\S+@\S+\.\S+/.test(email)
}

export function validatePasswordBasics(password: string) {
  const chars = Array.from(password)
  if (chars.length < 12 || chars.length > 128) return passwordRequirementText

  const hasLetter = chars.some(char => /\p{L}/u.test(char))
  const hasDigit = chars.some(char => /\p{Nd}/u.test(char))
  if (!hasLetter || !hasDigit) return passwordRequirementText

  const hasForbiddenControl = chars.some(char => {
    if (char === ' ') return false
    return /\p{Cc}/u.test(char)
  })
  if (hasForbiddenControl) return 'Пароль не должен содержать управляющие символы'

  return undefined
}

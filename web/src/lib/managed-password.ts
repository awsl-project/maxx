export const COMMON_PASSWORD_PUNCTUATION_PATTERN = /[!@#$%^&*?.,_\-+=/\\]/;
export const PASSWORD_POLICY_VIOLATION_CODE = 'PASSWORD_POLICY_VIOLATION';

export interface ManagedPasswordRuleState {
  minLength: boolean;
  hasNumber: boolean;
  hasLetter: boolean;
  hasPunctuation: boolean;
  categoryCount: number;
  isValid: boolean;
}

export function getManagedPasswordRuleState(password: string): ManagedPasswordRuleState {
  const checks = {
    minLength: Array.from(password).length >= 8,
    hasNumber: /\d/.test(password),
    hasLetter: /[A-Za-z]/.test(password),
    hasPunctuation: COMMON_PASSWORD_PUNCTUATION_PATTERN.test(password),
  };
  const categoryCount = [
    checks.minLength,
    checks.hasNumber,
    checks.hasLetter,
    checks.hasPunctuation,
  ].filter(Boolean).length;

  return {
    ...checks,
    categoryCount,
    isValid: categoryCount === 4,
  };
}

export function getManagedPasswordError(password: string, invalidMessage: string) {
  if (!password) {
    return undefined;
  }

  if (!getManagedPasswordRuleState(password).isValid) {
    return invalidMessage;
  }

  return undefined;
}

export function isPasswordPolicyViolationResponse(data?: { code?: string }) {
  return data?.code === PASSWORD_POLICY_VIOLATION_CODE;
}

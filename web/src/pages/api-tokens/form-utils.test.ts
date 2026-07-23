import { describe, expect, it } from 'vitest';
import { buildAPITokenReactivatePayload, buildAPITokenUpdatePayload } from './form-utils';

describe('buildAPITokenUpdatePayload', () => {
  it('does not clear expiration when the date field was not touched', () => {
    const payload = buildAPITokenUpdatePayload({
      name: 'Existing token',
      description: 'updated description',
      projectID: '0',
      devMode: false,
      expiresAt: '',
      expiresAtTouched: false,
    });

    expect(payload).not.toHaveProperty('expiresAt');
  });

  it('sends an empty expiration when reset expiration is requested', () => {
    const payload = buildAPITokenUpdatePayload({
      name: 'Expired token',
      description: '',
      projectID: '7',
      devMode: true,
      expiresAt: '',
      expiresAtTouched: true,
    });

    expect(payload).toMatchObject({
      name: 'Expired token',
      description: '',
      projectID: 7,
      devMode: true,
      expiresAt: '',
    });
  });

  it('serializes a touched future expiration date as RFC3339', () => {
    const payload = buildAPITokenUpdatePayload({
      name: 'Future token',
      description: '',
      projectID: '0',
      devMode: false,
      expiresAt: '2026-07-20',
      expiresAtTouched: true,
    });

    expect(payload.expiresAt).toBe(new Date('2026-07-20').toISOString());
  });
});

it('builds a validity reset payload', () => {
  expect(buildAPITokenReactivatePayload()).toEqual({
    isEnabled: true,
    expiresAt: '',
    resetValidity: true,
  });
});

import type { APITokenUpdateData } from '@/lib/transport';

type BuildAPITokenUpdatePayloadInput = {
  name: string;
  description: string;
  projectID: string;
  devMode: boolean;
  expiresAt: string;
  expiresAtTouched: boolean;
};

export function buildAPITokenUpdatePayload({
  name,
  description,
  projectID,
  devMode,
  expiresAt,
  expiresAtTouched,
}: BuildAPITokenUpdatePayloadInput): APITokenUpdateData {
  const payload: APITokenUpdateData = {
    name,
    description,
    projectID: parseInt(projectID) || 0,
    devMode,
  };

  if (expiresAtTouched) {
    payload.expiresAt = expiresAt ? new Date(expiresAt).toISOString() : '';
  }

  return payload;
}

export function buildAPITokenReactivatePayload(): APITokenUpdateData {
  return {
    isEnabled: true,
    expiresAt: '',
    resetValidity: true,
  };
}

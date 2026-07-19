import type { APIToken } from '@/lib/transport';

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
}: BuildAPITokenUpdatePayloadInput): Partial<APIToken> {
  const payload: Partial<APIToken> = {
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

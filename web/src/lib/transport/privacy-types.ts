export type PrivacyAction = 'mask' | 'warn' | 'block';
export interface PrivacyRule {
  kind: string;
  prefix?: string;
  regex?: string;
}
export interface PrivacyConfig {
  enabled: boolean;
  secrets: boolean;
  personal: boolean;
  kinds?: Record<string, boolean>;
  words?: string[];
  wordsAnyCase: boolean;
  wordsWhole: boolean;
  rules?: PrivacyRule[];
  actions?: Record<string, PrivacyAction>;
  allow?: string[];
  shape: boolean;
  mode: PrivacyAction;
  model: { enabled: boolean; name: 'astrlink-guard' | 'privacy-filter'; score: number };
  bypassProviders?: number[];
}
export interface PrivacyCategory {
  ID: string;
  Kinds: string[];
  On: boolean;
}
export interface PrivacyHit {
  Kind: string;
  By: string;
  Placeholder: string;
  Action: string;
  Head: string;
  Len: number;
}
export interface PrivacyPreview {
  body: string;
  hits: PrivacyHit[];
  blocked: boolean;
}
export interface PrivacyModel {
  model: string;
  supported: boolean;
  installed: boolean;
  size: number;
  downloading?: boolean;
  done?: number;
  total?: number;
  error?: string;
  running?: boolean;
  device?: string;
  loading?: boolean;
  waiting?: number;
  failed?: string;
}
export interface PrivacyModels {
  items: PrivacyModel[];
  canManage: boolean;
}
export interface PrivacyValue {
  placeholder: string;
  kind: string;
  by: string;
  hint?: string;
  length: number;
  count: number;
  action: PrivacyAction;
  restored: number;
}
export interface PrivacyRecord {
  id: string;
  time: string;
  providerId: number;
  requestId: number;
  status: string;
  bypassed: boolean;
  tookUs: number;
  values: PrivacyValue[];
}
export interface PrivacyRecords {
  items: PrivacyRecord[];
  total: number;
  page: number;
  limit: number;
}
export interface PrivacySummary {
  Requests: number;
  Hits: number;
  Restored: number;
  Masked: number;
  Warned: number;
  Blocked: number;
  Bypassed: number;
  Kinds: Record<string, number>;
  Finders: Record<string, number>;
  Providers: Record<number, number>;
}

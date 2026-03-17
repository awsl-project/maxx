import { createContext, useContext, useState } from 'react';
import type { ReactNode } from 'react';
import type { ClientType } from '@/lib/transport';
import { defaultClients, type ProviderFormData, type ClientConfig } from '../types';

interface ProviderFormContextType {
  // Form data
  formData: ProviderFormData;
  updateFormData: (updates: Partial<ProviderFormData>) => void;
  updateClient: (clientId: ClientType, updates: Partial<ClientConfig>) => void;
  resetFormData: () => void;

  // Validation
  isValid: () => boolean;

  // Loading states
  isSaving: boolean;
  setSaving: (saving: boolean) => void;
  saveStatus: 'idle' | 'success' | 'error';
  setSaveStatus: (status: 'idle' | 'success' | 'error') => void;

  // Validation results (for Antigravity/Kiro)
  validationResult: any | null;
  setValidationResult: (result: any | null) => void;

  // OAuth state (for Antigravity)
  oauthState: any | null;
  setOAuthState: (state: any | null) => void;
}

const ProviderFormContext = createContext<ProviderFormContextType | null>(null);

const initialFormData: ProviderFormData = {
  type: 'custom',
  name: '',
  selectedTemplate: null,
  baseURL: '',
  apiKey: '',
  clients: [...defaultClients],
  cloakMode: 'auto',
  cloakStrictMode: false,
  cloakSensitiveWords: '',
  disableErrorCooldown: false,
  excludeFromExport: false,
};

export function ProviderFormProvider({ children }: { children: ReactNode }) {
  const [formData, setFormData] = useState<ProviderFormData>(initialFormData);
  const [isSaving, setSaving] = useState(false);
  const [saveStatus, setSaveStatus] = useState<'idle' | 'success' | 'error'>('idle');
  const [validationResult, setValidationResult] = useState<any | null>(null);
  const [oauthState, setOAuthState] = useState<any | null>(null);

  const updateFormData = (updates: Partial<ProviderFormData>) => {
    setFormData((prev) => ({ ...prev, ...updates }));
  };

  const updateClient = (clientId: ClientType, updates: Partial<ClientConfig>) => {
    setFormData((prev) => ({
      ...prev,
      clients: prev.clients.map((c) => (c.id === clientId ? { ...c, ...updates } : c)),
    }));
  };

  const resetFormData = () => {
    setFormData(initialFormData);
    setSaveStatus('idle');
    setValidationResult(null);
    setOAuthState(null);
  };

  const isValid = (): boolean => {
    if (!formData.name.trim()) return false;
    if (!formData.apiKey.trim()) return false;
    const hasEnabledClient = formData.clients.some((c) => c.enabled);
    const hasUrl =
      !!formData.baseURL.trim() || formData.clients.some((c) => c.enabled && c.urlOverride.trim());
    return hasEnabledClient && !!hasUrl;
  };

  const value: ProviderFormContextType = {
    formData,
    updateFormData,
    updateClient,
    resetFormData,
    isValid,
    isSaving,
    setSaving,
    saveStatus,
    setSaveStatus,
    validationResult,
    setValidationResult,
    oauthState,
    setOAuthState,
  };

  return <ProviderFormContext.Provider value={value}>{children}</ProviderFormContext.Provider>;
}

export function useProviderForm() {
  const context = useContext(ProviderFormContext);
  if (!context) {
    throw new Error('useProviderForm must be used within ProviderFormProvider');
  }
  return context;
}

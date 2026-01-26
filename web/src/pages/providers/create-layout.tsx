import { Routes, Route } from 'react-router-dom';
import { ProviderFormProvider } from './context/provider-form-context';
import { SelectTypeStep } from './components/select-type-step';
import { AntigravityTokenImport } from './components/antigravity-token-import';
import { KiroTokenImport } from './components/kiro-token-import';
import { CodexTokenImport } from './components/codex-token-import';
import { CustomConfigStep } from './components/custom-config-step';

export function ProviderCreateLayout() {
  return (
    <ProviderFormProvider>
      <Routes>
        <Route index element={<SelectTypeStep />} />
        <Route path="custom" element={<CustomConfigStep />} />
        <Route path="antigravity" element={<AntigravityTokenImport />} />
        <Route path="kiro" element={<KiroTokenImport />} />
        <Route path="codex" element={<CodexTokenImport />} />
      </Routes>
    </ProviderFormProvider>
  );
}

import { Routes, Route } from 'react-router-dom';
import { ProviderFormProvider } from './context/provider-form-context';
import { SelectTypeStep } from './components/select-type-step';
import { AntigravityTokenImport } from './components/antigravity-token-import';
import { KiroTokenImport } from './components/kiro-token-import';
import { CodexTokenImport } from './components/codex-token-import';
import { GrokTokenImport } from './components/grok-token-import';
import { ClaudeTokenImport } from './components/claude-token-import';
import { CustomConfigStep } from './components/custom-config-step';
import { BedrockConfigStep } from './components/bedrock-config-step';
import { OpenRouterConfigStep } from './components/openrouter-config-step';
import { ZaiConfigStep } from './components/zai-config-step';
import { NewApiConfigStep } from './components/newapi-config-step';
import { OllamaConfigStep } from './components/ollama-config-step';
import { FalConfigStep } from './components/fal-config-step';

export function ProviderCreateLayout() {
  return (
    <ProviderFormProvider>
      <Routes>
        <Route index element={<SelectTypeStep />} />
        <Route path="custom" element={<CustomConfigStep />} />
        <Route path="bedrock" element={<BedrockConfigStep />} />
        <Route path="openrouter" element={<OpenRouterConfigStep />} />
        <Route path="zai" element={<ZaiConfigStep />} />
        <Route path="newapi" element={<NewApiConfigStep />} />
        <Route path="ollama" element={<OllamaConfigStep />} />
        <Route path="fal" element={<FalConfigStep />} />
        <Route path="antigravity" element={<AntigravityTokenImport />} />
        <Route path="kiro" element={<KiroTokenImport />} />
        <Route path="codex" element={<CodexTokenImport />} />
        <Route path="grok" element={<GrokTokenImport />} />
        <Route path="claude" element={<ClaudeTokenImport />} />
      </Routes>
    </ProviderFormProvider>
  );
}

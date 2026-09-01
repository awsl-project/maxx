import { useNavigate } from 'react-router-dom';
import type { ProviderTypeKey } from '../types';

export const PROVIDER_CREATE_PATHS: Record<ProviderTypeKey, string> = {
  custom: '/providers/create/custom',
  antigravity: '/providers/create/antigravity',
  kiro: '/providers/create/kiro',
  codex: '/providers/create/codex',
  claude: '/providers/create/claude',
  bedrock: '/providers/create/bedrock',
  openrouter: '/providers/create/openrouter',
  zai: '/providers/create/zai',
  newapi: '/providers/create/newapi',
  ollama: '/providers/create/ollama',
  grok: '/providers/create/grok',
  fal: '/providers/create/fal',
};

export function getProviderCreatePath(type: ProviderTypeKey): string {
  return PROVIDER_CREATE_PATHS[type];
}

export function useProviderNavigation() {
  const navigate = useNavigate();

  return {
    goToSelectType: () => navigate('/providers/create'),
    goToCustomConfig: () => navigate(PROVIDER_CREATE_PATHS.custom),
    goToAntigravity: () => navigate(PROVIDER_CREATE_PATHS.antigravity),
    goToKiro: () => navigate(PROVIDER_CREATE_PATHS.kiro),
    goToCodex: () => navigate(PROVIDER_CREATE_PATHS.codex),
    goToClaude: () => navigate(PROVIDER_CREATE_PATHS.claude),
    goToBedrock: () => navigate(PROVIDER_CREATE_PATHS.bedrock),
    goToOpenRouter: () => navigate(PROVIDER_CREATE_PATHS.openrouter),
    goToZai: () => navigate(PROVIDER_CREATE_PATHS.zai),
    goToNewApi: () => navigate(PROVIDER_CREATE_PATHS.newapi),
    goToOllama: () => navigate(PROVIDER_CREATE_PATHS.ollama),
    goToGrok: () => navigate(PROVIDER_CREATE_PATHS.grok),
    goToFal: () => navigate(PROVIDER_CREATE_PATHS.fal),
    goToProviderConfig: (type: ProviderTypeKey) => navigate(getProviderCreatePath(type)),
    goToProviders: () => navigate('/providers'),
    goBack: () => navigate(-1),
  };
}

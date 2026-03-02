import { useNavigate } from 'react-router-dom';

export function useProviderNavigation() {
  const navigate = useNavigate();

  return {
    goToSelectType: () => navigate('/providers/create'),
    goToCustomConfig: () => navigate('/providers/create/custom'),
    goToAntigravity: () => navigate('/providers/create/antigravity'),
    goToKiro: () => navigate('/providers/create/kiro'),
    goToCodex: () => navigate('/providers/create/codex'),
    goToClaude: () => navigate('/providers/create/claude'),
    goToProviders: () => navigate('/providers'),
    goBack: () => navigate(-1),
  };
}

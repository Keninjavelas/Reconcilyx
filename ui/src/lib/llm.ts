const providerStorageKey = "reconcilyx_llm_provider";
const keyStorageKey = "reconcilyx_llm_key";

export const LLM_PROVIDERS = ["ollama", "openai", "anthropic", "groq", "cohere"] as const;

export type LlmProvider = (typeof LLM_PROVIDERS)[number];

export function getLlmConfig(): { provider: LlmProvider; apiKey: string } {
  const stored = localStorage.getItem(providerStorageKey);
  const provider = LLM_PROVIDERS.includes(stored as LlmProvider)
    ? (stored as LlmProvider)
    : "ollama";
  return { provider, apiKey: localStorage.getItem(keyStorageKey) ?? "" };
}

export function setLlmConfig(provider: LlmProvider, apiKey: string): void {
  localStorage.setItem(providerStorageKey, provider);
  if (apiKey) {
    localStorage.setItem(keyStorageKey, apiKey);
  } else {
    localStorage.removeItem(keyStorageKey);
  }
  window.dispatchEvent(new Event("reconcilyx-llm-changed"));
}

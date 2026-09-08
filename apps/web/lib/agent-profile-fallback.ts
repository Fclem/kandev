export type AgentProfileFallbackState =
  | { kind: "none" }
  | { kind: "next" }
  | { kind: "model"; model: string };

export function classifyAgentProfileFallback(profile: {
  autoFallback?: boolean;
  fallbackModel?: string;
}): AgentProfileFallbackState {
  if (profile.autoFallback) return { kind: "next" };
  if (profile.fallbackModel) return { kind: "model", model: profile.fallbackModel };
  return { kind: "none" };
}

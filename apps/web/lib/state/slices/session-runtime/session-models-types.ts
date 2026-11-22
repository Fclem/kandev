import type { ModelCatalog } from "@kandev/types/model-catalog";

export type SessionModelEntry = {
  modelId: string;
  name: string;
  description?: string;
  usageMultiplier?: string;
  meta?: Record<string, unknown>;
};

export type ConfigOptionEntry = {
  type: string;
  id: string;
  name: string;
  description?: string;
  currentValue: string;
  category?: string;
  options?: { value: string; name: string; description?: string }[];
  /** Bounded provider group structure and verified composition bindings. */
  catalog?: ModelCatalog;
};

export type SessionModelsState = {
  bySessionId: Record<
    string,
    {
      currentModelId: string;
      models: SessionModelEntry[];
      configOptions: ConfigOptionEntry[];
      configOptionsSettled?: boolean;
      /** Provider-reported config values from the last settled snapshot. */
      confirmedConfigOptions?: Record<string, string>;
      confirmedConfigOptionsExecutionId?: string;
      pendingConfirmedConfigOptions?: {
        executionId: string;
        values: Record<string, string>;
      };
      configBaseline?: Record<string, string>;
      /** Marks the effective selector snapshot restored after explicit recovery. */
      settingsPolicy?: "provider_restored";
      /** Set when the session started on the profile's fallback model. */
      fallbackModel?: string;
      savedModelPreferenceId?: string;
    }
  >;
};

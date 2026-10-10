"use client";

import { useTranslation } from "react-i18next";
import { Label } from "@kandev/ui/label";
import { ModelCombobox } from "@/components/settings/model-combobox";
import { ModeCombobox } from "@/components/settings/mode-combobox";
import { ModelFallbackFields } from "./cli-profile-fallback-fields";
import type { AvailableAgent } from "@/lib/types/http";

export type ModelModeFieldsProps = {
  modelConfig: NonNullable<AvailableAgent["model_config"]> | null;
  model: string;
  fallbackModel: string;
  autoFallback: boolean;
  requireExactModel: boolean;
  limitFallback: boolean;
  resumeAfterReset: boolean;
  onLimitFallbackChange: (v: boolean) => void;
  onResumeAfterResetChange: (v: boolean) => void;
  mode: string;
  onModelChange: (v: string) => void;
  onFallbackModelChange: (v: string) => void;
  onAutoFallbackChange: (v: boolean) => void;
  onRequireExactModelChange: (v: boolean) => void;
  onModeChange: (v: string) => void;
};

export function ModelModeFields({
  modelConfig,
  model,
  fallbackModel,
  autoFallback,
  requireExactModel,
  limitFallback,
  resumeAfterReset,
  onLimitFallbackChange,
  onResumeAfterResetChange,
  mode,
  onModelChange,
  onFallbackModelChange,
  onAutoFallbackChange,
  onRequireExactModelChange,
  onModeChange,
}: ModelModeFieldsProps) {
  const { t } = useTranslation();
  if (!modelConfig) {
    return <p className="text-xs text-muted-foreground">{t("common:pickACliClientToLoad")}</p>;
  }
  const availableModels = modelConfig.available_models ?? [];
  const startModelGone = Boolean(model && !availableModels.some((m) => m.id === model));
  const fallbackModelGone = Boolean(
    fallbackModel && !availableModels.some((m) => m.id === fallbackModel),
  );
  return (
    <div className="grid grid-cols-1 gap-4">
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
        <div>
          <Label>{t("common:model")}</Label>
          <ModelCombobox
            value={model}
            onChange={onModelChange}
            models={
              startModelGone
                ? [
                    ...availableModels,
                    {
                      id: model,
                      name: `${model} (${t("settings:startModelUnavailable")})`,
                      disabled: true,
                    },
                  ]
                : availableModels
            }
            currentModelId={modelConfig.current_model_id}
          />
        </div>
        {(modelConfig.available_modes ?? []).length > 0 && (
          <div>
            <Label>{t("common:mode")}</Label>
            <ModeCombobox
              value={mode}
              onChange={onModeChange}
              modes={modelConfig.available_modes ?? []}
              currentModeId={modelConfig.current_mode_id}
            />
          </div>
        )}
      </div>
      <ModelFallbackFields
        availableModels={availableModels}
        fallbackModel={fallbackModel}
        fallbackModelGone={fallbackModelGone}
        autoFallback={autoFallback}
        requireExactModel={requireExactModel}
        limitFallback={limitFallback}
        resumeAfterReset={resumeAfterReset}
        onLimitFallbackChange={onLimitFallbackChange}
        onResumeAfterResetChange={onResumeAfterResetChange}
        currentModelId={modelConfig.current_model_id}
        onFallbackModelChange={onFallbackModelChange}
        onAutoFallbackChange={onAutoFallbackChange}
        onRequireExactModelChange={onRequireExactModelChange}
      />
    </div>
  );
}

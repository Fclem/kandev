"use client";

import { useEffect, useState } from "react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { useTranslation } from "react-i18next";

type ScheduledConfigProps = {
  config: Record<string, unknown>;
  onUpdate: (config: Record<string, unknown>) => void;
};

const PRESETS = [
  { labelKey: "automations:everyHour", expression: "@hourly" },
  { labelKey: "automations:everyDay", expression: "@daily" },
  { labelKey: "automations:everyWeek", expression: "@weekly" },
] as const;

export function ScheduledConfig({ config, onUpdate }: ScheduledConfigProps) {
  const { t } = useTranslation();
  const configExpr = (config.cron_expression as string) ?? "";
  const [cronExpression, setCronExpression] = useState(configExpr);
  useEffect(() => {
    setCronExpression(configExpr);
  }, [configExpr]);

  const handlePreset = (expression: string) => {
    setCronExpression(expression);
    onUpdate({ ...config, cron_expression: expression });
  };

  const handleCustomChange = (value: string) => {
    setCronExpression(value);
  };

  const handleCustomBlur = () => {
    onUpdate({ ...config, cron_expression: cronExpression });
  };

  return (
    <div className="space-y-3">
      <div className="flex gap-2">
        {PRESETS.map((preset) => (
          <Button
            key={preset.expression}
            variant={cronExpression === preset.expression ? "secondary" : "outline"}
            size="sm"
            className="cursor-pointer"
            onClick={() => handlePreset(preset.expression)}
          >
            {t(preset.labelKey)}
          </Button>
        ))}
      </div>
      <div className="space-y-1.5">
        <Label className="text-xs">{t("automations:cronExpression")}</Label>
        <Input
          value={cronExpression}
          onChange={(e) => handleCustomChange(e.target.value)}
          onBlur={handleCustomBlur}
          placeholder="*/5 * * * *"
          className="font-mono text-sm"
        />
        <p className="text-xs text-muted-foreground">
          {t("automations:standard5FieldCronOrShortcuts")}
        </p>
      </div>
    </div>
  );
}

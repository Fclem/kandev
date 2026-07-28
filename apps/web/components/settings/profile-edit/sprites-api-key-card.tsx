"use client";
import { useState, useCallback } from "react";
import { useTranslation } from "react-i18next";
import { IconTestPipe, IconLoader2, IconCheck, IconX, IconSparkles } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { CardContent, CardHeader, CardTitle, CardDescription } from "@kandev/ui/card";
import { testSpritesConnection } from "@/lib/api/domains/sprites-api";
import { useSprites } from "@/hooks/domains/settings/use-sprites";
import { InlineSecretSelect } from "@/components/settings/profile-edit/inline-secret-select";
import type { SecretListItem } from "@/lib/types/http-secrets";
import type { SpritesTestResult, SpritesTestStep } from "@/lib/types/http-sprites";
import { SettingsCard } from "@/components/settings/settings-card";

type SpritesApiKeyCardProps = {
  secretId: string | null;
  baselineSecretId?: string | null;
  onSecretIdChange: (id: string | null) => void;
  secrets: SecretListItem[];
};

export function SpritesApiKeyCard({
  secretId,
  baselineSecretId,
  onSecretIdChange,
  secrets,
}: SpritesApiKeyCardProps) {
  const { status } = useSprites(secretId ?? undefined);
  const { t } = useTranslation();
  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<SpritesTestResult | null>(null);

  const handleTest = useCallback(async () => {
    if (!secretId) return;
    setTesting(true);
    setTestResult(null);
    try {
      const result = await testSpritesConnection(secretId);
      setTestResult(result);
    } catch {
      setTestResult({
        success: false,
        steps: [],
        total_duration_ms: 0,
        sprite_name: "",
        error: t("settings:failedToConnectToBackend"),
      });
    } finally {
      setTesting(false);
    }
  }, [secretId, t]);

  const isDirty = baselineSecretId !== undefined && secretId !== baselineSecretId;
  return (
    <SettingsCard isDirty={isDirty}>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div>
            <CardTitle className="flex items-center gap-2">
              <IconSparkles className="h-5 w-5" />
              {t("settings:apiKey")}
            </CardTitle>
            <CardDescription>{t("settings:selectTheSecretContainingYourSprites")}</CardDescription>
          </div>
          <ConnectionBadge secretId={secretId} status={status} />
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        <InlineSecretSelect
          secretId={secretId}
          onSecretIdChange={onSecretIdChange}
          secrets={secrets}
          label={t("settings:secret")}
          isDirty={isDirty}
        />
        {secretId && (
          <ConnectionDetails
            status={status}
            testing={testing}
            testResult={testResult}
            onTest={handleTest}
          />
        )}
      </CardContent>
    </SettingsCard>
  );
}

function ConnectionBadge({
  secretId,
  status,
}: {
  secretId: string | null;
  status: ReturnType<typeof useSprites>["status"];
}) {
  const { t } = useTranslation();
  if (!secretId) {
    return <Badge variant="secondary">{t("settings:notConfigured")}</Badge>;
  }
  if (status?.connected) {
    return (
      <Badge variant="default" className="bg-green-600">
        {t("common:connected")}
      </Badge>
    );
  }
  if (status?.token_configured) {
    return <Badge variant="destructive">{t("settings:disconnected")}</Badge>;
  }
  return <Badge variant="secondary">{t("settings:checking")}</Badge>;
}

function ConnectionDetails({
  status,
  testing,
  testResult,
  onTest,
}: {
  status: ReturnType<typeof useSprites>["status"];
  testing: boolean;
  testResult: SpritesTestResult | null;
  onTest: () => void;
}) {
  const { t } = useTranslation();
  return (
    <>
      <ConnectionStatusText status={status} />
      <Button
        variant="outline"
        size="sm"
        onClick={onTest}
        disabled={testing}
        className="cursor-pointer"
      >
        {testing ? (
          <IconLoader2 className="mr-1.5 h-4 w-4 animate-spin" />
        ) : (
          <IconTestPipe className="mr-1.5 h-4 w-4" />
        )}
        {t("settings:testConnection")}
      </Button>
      {testResult && <TestResultDisplay result={testResult} />}
    </>
  );
}

function ConnectionStatusText({ status }: { status: ReturnType<typeof useSprites>["status"] }) {
  const { t } = useTranslation();
  if (status?.connected) {
    const count = status.instance_count;
    return (
      <p className="text-sm text-muted-foreground">
        {t("settings:connectedActiveSprites", { count: count })}
      </p>
    );
  }
  if (status?.token_configured) {
    return (
      <p className="text-sm text-muted-foreground">
        {t("settings:tokenConfiguredButUnableToConnect")}
      </p>
    );
  }
  return <p className="text-sm text-muted-foreground">{t("settings:verifyingConnection")}</p>;
}

function TestResultDisplay({ result }: { result: SpritesTestResult }) {
  const { t } = useTranslation();
  return (
    <div className="rounded-md border p-3 space-y-2">
      <div className="flex items-center gap-2 text-sm font-medium">
        {result.success ? (
          <IconCheck className="h-4 w-4 text-green-600" />
        ) : (
          <IconX className="h-4 w-4 text-red-600" />
        )}
        {result.success ? t("settings:connectionTestPassed") : t("settings:connectionTestFailed")}
        <span className="text-muted-foreground font-normal">({result.total_duration_ms}ms)</span>
      </div>
      {result.steps.map((step: SpritesTestStep) => (
        <StepRow key={step.name} step={step} />
      ))}
      {result.error && !result.steps.some((s) => s.error) && (
        <p className="text-sm text-red-600">{result.error}</p>
      )}
    </div>
  );
}

function StepRow({ step }: { step: SpritesTestStep }) {
  return (
    <div className="flex items-center gap-2 text-sm pl-2">
      {step.success ? (
        <IconCheck className="h-3 w-3 text-green-600 shrink-0" />
      ) : (
        <IconX className="h-3 w-3 text-red-600 shrink-0" />
      )}
      <span>{step.name}</span>
      <span className="text-muted-foreground">({step.duration_ms}ms)</span>
      {step.error && <span className="text-red-600 truncate">{step.error}</span>}
    </div>
  );
}

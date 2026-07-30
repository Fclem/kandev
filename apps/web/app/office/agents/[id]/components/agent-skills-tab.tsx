"use client";

import { useEffect, useState, useCallback } from "react";
import Link from "@/components/routing/app-link";
import { toast } from "sonner";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Checkbox } from "@kandev/ui/checkbox";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useAppStore } from "@/components/state-provider";
import { listSkills, updateAgentProfile } from "@/lib/api/domains/office-api";
import type { AgentProfile, Skill } from "@/lib/state/slices/office/types";
import { Trans, useTranslation } from "react-i18next";

type AgentSkillsTabProps = {
  agent: AgentProfile;
};

/**
 * Hydrate the office skills store on mount. The workspace Skills page
 * populates it as a side effect of viewing, but a user landing
 * directly on /office/agents/<id>/skills wouldn't have run that path
 * yet. Hitting listSkills also triggers the backend's lazy per-
 * workspace system-skill sync, so a fresh workspace shows the
 * bundled set on first visit.
 */
function useHydrateSkills() {
  const setSkills = useAppStore((s) => s.setSkills);
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  useEffect(() => {
    if (!workspaceId) return;
    let cancelled = false;
    listSkills(workspaceId)
      .then((res) => {
        if (!cancelled) setSkills(res.skills ?? []);
      })
      .catch(() => {
        // Non-fatal: existing store contents (possibly empty) render
        // the "No skills registered" CTA, which still lets the user
        // pivot to the Skills page.
      });
    return () => {
      cancelled = true;
    };
  }, [workspaceId, setSkills]);
}

// SkillRow renders one toggleable skill entry with its system/default badges.
function SkillRow({
  skill,
  role,
  checked,
  isDefault,
  onToggle,
}: {
  skill: Skill;
  role: string;
  checked: boolean;
  isDefault: boolean;
  onToggle: () => void;
}) {
  const { t } = useTranslation();
  return (
    <label
      data-testid={`skill-toggle-${skill.slug}`}
      className="flex items-center gap-3 py-1.5 px-2 rounded-md hover:bg-accent/50 cursor-pointer"
    >
      <Checkbox
        checked={checked}
        onCheckedChange={onToggle}
        className="cursor-pointer"
        data-testid={`skill-toggle-checkbox-${skill.slug}`}
      />
      <span className="text-sm">{skill.name}</span>
      {skill.isSystem && (
        <Tooltip>
          <TooltipTrigger asChild>
            <Badge variant="outline" className="text-[10px] text-muted-foreground">
              {t("common:system")}
            </Badge>
          </TooltipTrigger>
          <TooltipContent>
            <Trans
              i18nKey="office:bundledWithKandev"
              values={{ value1: skill.systemVersion ? ` v${skill.systemVersion}` : "" }}
            >
              {t("office:bundledWithKandev2")}
              {skill.systemVersion ? ` v${skill.systemVersion}` : ""}
            </Trans>
          </TooltipContent>
        </Tooltip>
      )}
      {isDefault && (
        <Tooltip>
          <TooltipTrigger asChild>
            <span className="text-[10px] text-muted-foreground">
              <Trans i18nKey="office:defaultFor" values={{ role }}>
                default for {role}
              </Trans>
            </span>
          </TooltipTrigger>
          <TooltipContent>
            <Trans i18nKey="office:thisSkillIsAutoAttachedTo" values={{ role }}>
              This skill is auto-attached to new {role} agents. You can still untick it for this
              agent.
            </Trans>
          </TooltipContent>
        </Tooltip>
      )}
      <span className="text-xs text-muted-foreground ml-auto">{skill.slug}</span>
    </label>
  );
}

export function AgentSkillsTab({ agent }: AgentSkillsTabProps) {
  const { t } = useTranslation();
  useHydrateSkills();
  const skills = useAppStore((s) => s.office.skills);
  const updateStore = useAppStore((s) => s.updateOfficeAgentProfile);
  const [skillIds, setSkillIds] = useState<string[]>(agent.skillIds ?? []);
  const [saving, setSaving] = useState(false);
  const [dirty, setDirty] = useState(false);

  const toggle = useCallback((id: string) => {
    setSkillIds((prev) => {
      const next = prev.includes(id) ? prev.filter((s) => s !== id) : [...prev, id];
      setDirty(true);
      return next;
    });
  }, []);

  const handleSave = useCallback(async () => {
    setSaving(true);
    try {
      await updateAgentProfile(agent.id, { skillIds });
      updateStore(agent.id, { skillIds });
      setDirty(false);
      toast.success(t("office:skillsUpdated"));
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to update skills");
    } finally {
      setSaving(false);
    }
  }, [agent.id, skillIds, updateStore]);

  if (skills.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-12 gap-3">
        <p className="text-sm text-muted-foreground">{t("office:noSkillsRegisteredYet")}</p>
        <Button asChild variant="outline" size="sm" className="cursor-pointer">
          <Link href="/office/workspace/skills">{t("office:manageSkillsInCompany")}</Link>
        </Button>
      </div>
    );
  }

  const selected = new Set(skillIds);

  return (
    <div className="space-y-4 mt-4">
      <p className="text-xs text-muted-foreground">{t("office:skillsThisAgentOwnsSkillsAre")}</p>
      <div className="space-y-1.5">
        {skills.map((skill) => (
          <SkillRow
            key={skill.id}
            skill={skill}
            role={agent.role}
            checked={selected.has(skill.id)}
            isDefault={Boolean(
              skill.isSystem && (skill.defaultForRoles ?? []).includes(agent.role),
            )}
            onToggle={() => toggle(skill.id)}
          />
        ))}
      </div>
      {dirty && (
        <div className="flex justify-end">
          <Button onClick={handleSave} disabled={saving} className="cursor-pointer">
            {saving ? "Saving..." : "Save skills"}
          </Button>
        </div>
      )}
    </div>
  );
}

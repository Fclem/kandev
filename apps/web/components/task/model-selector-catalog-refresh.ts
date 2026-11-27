import { t } from "@/lib/i18n";
import type { ModelCatalog, ModelCatalogNode } from "@kandev/types/model-catalog";
import { isModelConfigOption } from "@/components/model-config-selector";
import type { ModelSelectorOption, SelectConfigOption } from "@/components/model-config-selector";
import type { TaskSession } from "@/lib/types/http";
import {
  projectedSessionProviderCatalog,
  sameSessionCatalogOwner,
} from "@/lib/types/session-catalog";
import type {
  SessionCatalogOwner,
  SessionCatalogRecord,
  SessionProviderCatalog,
} from "@/lib/types/session-catalog";

type Input = {
  session: TaskSession | undefined;
  record: SessionCatalogRecord | undefined;
  sourceOwner: SessionCatalogOwner | undefined;
  provider: SessionProviderCatalog | undefined;
  modelOptions: ModelSelectorOption[];
  configOptions: SelectConfigOption[];
  currentModel: string | null;
  providerModelIds: Set<string>;
};

function projectModelConfig(
  configOptions: SelectConfigOption[],
  models: ModelSelectorOption[],
  currentModel: string | null,
  catalog: ModelCatalog | undefined,
): SelectConfigOption[] {
  const options = models
    .filter((choice) => !choice.disabled)
    .map((choice) => ({
      value: choice.id,
      name: choice.name,
      description: choice.description,
    }));
  if (!options.length) return configOptions.filter((option) => !isModelConfigOption(option));
  const previous = configOptions.find(isModelConfigOption);
  const model: SelectConfigOption = {
    ...previous,
    type: "select",
    id: previous?.id ?? "model",
    category: "model",
    name: previous?.name ?? t("common:model"),
    currentValue: previous?.currentValue ?? currentModel ?? "",
    options,
    catalog,
  };
  return previous
    ? configOptions.map((option) => (isModelConfigOption(option) ? model : option))
    : [model, ...configOptions];
}

function projectProviderModelChoices(input: Input) {
  const { session, sourceOwner, currentModel } = input;
  const projected = projectedSessionProviderCatalog(
    input.provider,
    session?.execution_profile_id || session?.agent_profile_id,
    sourceOwner,
  );
  if (!projected) return input;
  const { record: provider, current } = projected;
  const surface = provider.result.models;
  const advertised =
    current && provider.result.outcome === "complete" && surface.present && surface.supported;
  const modelOptions: ModelSelectorOption[] = advertised
    ? (surface.choices ?? []).map((choice) => ({
        id: choice.id,
        name: choice.name || choice.id,
        description: choice.description,
        source: "acp",
      }))
    : [];
  const providerModelIds = new Set(modelOptions.map((choice) => choice.id));
  if (currentModel && !providerModelIds.has(currentModel)) {
    const previous = input.modelOptions.find((choice) => choice.id === currentModel);
    modelOptions.push({
      id: currentModel,
      name: previous?.name ?? currentModel,
      disabled: true,
      disabledReason: t("task:sessionCatalogChoiceUnavailable"),
    });
  }
  const configOptions = projectModelConfig(
    input.configOptions,
    modelOptions,
    currentModel,
    advertised ? surface.catalog : undefined,
  );
  return { ...input, modelOptions, configOptions, providerModelIds };
}

function projectHostNavigation(
  record: SessionCatalogRecord,
  models: ModelSelectorOption[],
  providerIds: Set<string>,
  refreshIds: Set<string>,
): ModelCatalog {
  const groupIds = new Set<string>();
  const values = new Set<string>();
  const modelsById = new Map<string, ModelSelectorOption>();
  for (const model of models) modelsById.set(model.id, model);
  const refreshGroups = new Set(
    (record.catalog?.compositions ?? [])
      .filter((composition) =>
        composition.bindings.every((binding) => refreshIds.has(binding.model)),
      )
      .map((composition) => composition.group_id),
  );
  const decorate = (nodes: ModelCatalogNode[]): ModelCatalogNode[] =>
    nodes.map((node) => {
      if (node.id) groupIds.add(node.id);
      if (node.value) values.add(node.value);
      const requiresRefresh =
        node.kind === "choice"
          ? !!node.value && refreshIds.has(node.value)
          : !!node.id && refreshGroups.has(node.id);
      return {
        ...node,
        description: requiresRefresh
          ? [node.description, t("task:sessionCatalogChoiceRequiresRefresh")]
              .filter(Boolean)
              .join(". ")
          : node.description,
        ...(node.children ? { children: decorate(node.children) } : {}),
      };
    });
  const nodes = decorate(record.catalog?.nodes ?? []);
  for (const choice of record.models ?? []) {
    if (values.has(choice.id)) continue;
    values.add(choice.id);
    const model = modelsById.get(choice.id);
    nodes.push({
      kind: "choice",
      value: choice.id,
      label: choice.name || choice.id,
      description: model?.description,
    });
  }
  const providerOnly = models.filter(
    (choice) => providerIds.has(choice.id) && !values.has(choice.id),
  );
  if (providerOnly.length) {
    let suffix = 0;
    let groupId = "session-provider-only";
    while (groupIds.has(groupId)) groupId = `session-provider-only-${++suffix}`;
    nodes.push({
      kind: "group",
      id: groupId,
      label: t("task:sessionCatalogProviderOnlyGroup"),
      children: providerOnly.map((choice) => ({
        kind: "choice",
        value: choice.id,
        label: choice.name,
        description: choice.description,
      })),
    });
  }
  return { version: 1, nodes, compositions: record.catalog?.compositions };
}

export function projectSessionModelChoices(input: Input) {
  const {
    session,
    record,
    sourceOwner,
    modelOptions,
    configOptions,
    currentModel,
    providerModelIds,
  } = projectProviderModelChoices(input);
  const refreshModelIds = new Set<string>();
  if (
    !session ||
    !record ||
    record.disposition !== "host_candidates" ||
    record.profile_id !== (session.execution_profile_id || session.agent_profile_id) ||
    !sameSessionCatalogOwner(record.owner, sourceOwner) ||
    !record.models?.length
  ) {
    return { modelOptions, configOptions, refreshModelIds };
  }
  const seen = new Set<string>();
  const candidates: ModelSelectorOption[] = record.models.flatMap<ModelSelectorOption>((choice) => {
    if (seen.has(choice.id)) return [];
    seen.add(choice.id);
    const needsRefresh = !providerModelIds.has(choice.id);
    if (needsRefresh) refreshModelIds.add(choice.id);
    return [
      {
        id: choice.id,
        name: choice.name || choice.id,
        source: "dynamic",
        requiresRefresh: needsRefresh,
        description: needsRefresh
          ? [choice.description, t("task:sessionCatalogChoiceRequiresRefresh")]
              .filter(Boolean)
              .join(". ")
          : choice.description,
      },
    ];
  });
  const combined = [
    ...candidates,
    ...modelOptions.filter(
      (choice) => !seen.has(choice.id) && (providerModelIds.has(choice.id) || choice.disabled),
    ),
  ];
  const catalog = projectHostNavigation(record, combined, providerModelIds, refreshModelIds);
  return {
    modelOptions: combined,
    configOptions: projectModelConfig(configOptions, combined, currentModel, catalog),
    refreshModelIds,
  };
}

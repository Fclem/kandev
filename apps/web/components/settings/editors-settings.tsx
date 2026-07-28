"use client";
import { useCallback, useEffect, useMemo } from "react";
import { useTranslation } from "react-i18next";
import { IconEdit, IconTrash } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Separator } from "@kandev/ui/separator";
import { SettingsPageTemplate } from "@/components/settings/settings-page-template";
import { Combobox, type ComboboxOption } from "@/components/combobox";
import { EditableCard } from "@/components/settings/editable-card";
import {
  EditorForm,
  type EditorFormState,
  defaultFormState,
  formStateFromEditor,
  getCustomEditorSummary,
} from "@/components/settings/editor-form";
import {
  LspLanguageCards,
  LspServerConfigSection,
} from "@/components/settings/editors-lsp-sections";
import type { EditorOption } from "@/lib/types/http";
import type { RequestStatus } from "@/lib/http/use-request";
import {
  useEditorsSettingsState,
  useLspConfigActions,
  useApplyEditors,
  useEditorRequests,
  useSaveRequest,
  buildDefaultEditorOptions,
  sortCustomEditors,
  resolveAvailableEditors,
  isCustomEditor,
  type EditorsSettingsState,
} from "@/components/settings/editors-settings-state";
import { isEditorsSettingsDirty } from "./settings-dirty";

type EditorRequestProps = { isLoading: boolean; status: RequestStatus };
type CreateReq = EditorRequestProps & { run: (state: EditorFormState) => Promise<void> };
type UpdateReq = EditorRequestProps & {
  run: (id: string, state: EditorFormState) => Promise<void>;
};
type DeleteReq = EditorRequestProps & { run: (id: string) => Promise<void> };

type CustomEditorsListProps = {
  customEditors: EditorOption[];
  editingId: string | null;
  setEditingId: (id: string | null) => void;
  isAdding: boolean;
  setIsAdding: (adding: boolean) => void;
  createRequest: CreateReq;
  updateRequest: UpdateReq;
  deleteRequest: DeleteReq;
};

type CustomEditorRowProps = {
  editor: EditorOption;
  editingId: string | null;
  setEditingId: (id: string | null) => void;
  updateRequest: UpdateReq;
  deleteRequest: DeleteReq;
};

function CustomEditorRow({
  editor,
  editingId,
  setEditingId,
  updateRequest,
  deleteRequest,
}: CustomEditorRowProps) {
  const { t } = useTranslation();
  const editorName = editor.name;
  return (
    <EditableCard
      key={editor.id}
      isEditing={editingId === editor.id}
      historyId={`editor-${editor.id}`}
      onOpen={() => setEditingId(editor.id)}
      onClose={() => setEditingId(null)}
      renderEdit={({ close }) => (
        <EditorForm
          title={t("settings:edit", { editorName })}
          initialState={formStateFromEditor(editor)}
          onCancel={close}
          onSave={(state) => updateRequest.run(editor.id, state)}
          onSaved={close}
          submitLabel={t("settings:saveChanges")}
          isSaving={updateRequest.isLoading}
          coordinatedSaveId={`custom-editor:${editor.id}`}
        />
      )}
      renderPreview={({ open }) => (
        <div
          className="rounded-lg border border-border/70 bg-background p-4 flex items-center justify-between gap-3 cursor-pointer"
          onClick={open}
        >
          <div className="min-w-0">
            <div className="font-medium text-sm text-foreground truncate">{editor.name}</div>
            <div className="text-xs text-muted-foreground truncate">
              {getCustomEditorSummary(editor)}
            </div>
          </div>
          <div className="flex items-center gap-2">
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="cursor-pointer"
              onClick={(event) => {
                event.stopPropagation();
                open();
              }}
            >
              <IconEdit className="h-4 w-4" />
              {t("common:edit")}
            </Button>
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="cursor-pointer"
              onClick={(event) => {
                event.stopPropagation();
                void deleteRequest.run(editor.id);
              }}
            >
              <IconTrash className="h-4 w-4" />
              {t("settings:remove")}
            </Button>
          </div>
        </div>
      )}
    />
  );
}

function CustomEditorsList({
  customEditors,
  editingId,
  setEditingId,
  isAdding,
  setIsAdding,
  createRequest,
  updateRequest,
  deleteRequest,
}: CustomEditorsListProps) {
  const { t } = useTranslation();
  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <div className="text-sm font-medium text-foreground">{t("settings:customEditors")}</div>
        <Button type="button" variant="outline" onClick={() => setIsAdding(true)}>
          {t("settings:addCustomEditor")}
        </Button>
      </div>
      {isAdding && (
        <EditorForm
          title={t("settings:newCustomEditor")}
          initialState={defaultFormState()}
          onCancel={() => setIsAdding(false)}
          onSave={(state) => createRequest.run(state)}
          onSaved={() => setIsAdding(false)}
          submitLabel={t("settings:addEditor")}
          isSaving={createRequest.isLoading}
          coordinatedSaveId="custom-editor:new"
          dirtyWhenMounted
        />
      )}
      <div className="space-y-3">
        {customEditors.length === 0 && !isAdding && (
          <div className="rounded-lg border border-dashed border-border/70 bg-muted/30 p-4 text-sm text-muted-foreground">
            {t("settings:noCustomEditorsYet")}
          </div>
        )}
        {customEditors.map((editor) => (
          <CustomEditorRow
            key={editor.id}
            editor={editor}
            editingId={editingId}
            setEditingId={setEditingId}
            updateRequest={updateRequest}
            deleteRequest={deleteRequest}
          />
        ))}
      </div>
    </div>
  );
}

type EditorsSectionProps = {
  defaultOptions: ComboboxOption[];
  defaultEditorId: string;
  baselineDefaultId: string;
  availableEditors: EditorOption[];
  builtInEditors: EditorOption[];
  onDefaultEditorChange: (value: string) => void;
  customEditors: EditorOption[];
  editingId: string | null;
  setEditingId: (id: string | null) => void;
  isAdding: boolean;
  setIsAdding: (adding: boolean) => void;
  createRequest: CreateReq;
  updateRequest: UpdateReq;
  deleteRequest: DeleteReq;
};

function EditorsSection({
  defaultOptions,
  defaultEditorId,
  baselineDefaultId,
  availableEditors,
  builtInEditors,
  onDefaultEditorChange,
  customEditors,
  editingId,
  setEditingId,
  isAdding,
  setIsAdding,
  createRequest,
  updateRequest,
  deleteRequest,
}: EditorsSectionProps) {
  const { t } = useTranslation();
  return (
    <div className="space-y-6">
      <div className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
        {t("settings:editors")}
      </div>
      <div className="space-y-2">
        <div className="text-sm font-medium text-foreground">{t("common:default")}</div>
        <div
          className="min-w-[280px] rounded-md border border-transparent"
          data-settings-dirty={defaultEditorId !== baselineDefaultId}
        >
          <Combobox
            options={defaultOptions}
            value={defaultEditorId}
            onValueChange={(value) => {
              if (!value) return;
              onDefaultEditorChange(value);
            }}
            placeholder={t("settings:selectADefaultEditor")}
            searchPlaceholder={t("settings:searchEditors")}
            emptyMessage={t("settings:noEditorFound")}
            disabled={availableEditors.length === 0}
          />
        </div>
      </div>
      <CustomEditorsList
        customEditors={customEditors}
        editingId={editingId}
        setEditingId={setEditingId}
        isAdding={isAdding}
        setIsAdding={setIsAdding}
        createRequest={createRequest}
        updateRequest={updateRequest}
        deleteRequest={deleteRequest}
      />
      {builtInEditors.length > 0 && (
        <div className="space-y-2">
          <div className="text-sm font-medium text-foreground">
            {t("settings:supportedEditors")}
          </div>
          <div className="grid gap-2 md:grid-cols-2">
            {builtInEditors.map((editor) => (
              <div
                key={editor.id}
                className="rounded-lg border border-border/60 bg-background px-3 py-2 flex items-center justify-between"
              >
                <span className="text-sm text-foreground truncate">{editor.name}</span>
                <Badge variant={editor.installed ? "secondary" : "outline"}>
                  {editor.installed ? t("settings:installed2") : t("settings:notInstalled")}
                </Badge>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

function getEditorsSaveRevision(state: EditorsSettingsState): string {
  return JSON.stringify({
    defaultEditorId: state.defaultEditorId,
    lspAutoStartLanguages: state.lspAutoStartLanguages,
    lspAutoInstallLanguages: state.lspAutoInstallLanguages,
    lspConfigStrings: state.lspConfigStrings,
  });
}

function useSyncEditors(editors: EditorOption[], setEditors: (editors: EditorOption[]) => void) {
  useEffect(() => setEditors(editors), [editors, setEditors]);
}

/** Add/remove a language id in the LSP auto-start and auto-install lists. */
function useLspLanguageToggles(
  setLspAutoStartLanguages: (fn: (prev: string[]) => string[]) => void,
  setLspAutoInstallLanguages: (fn: (prev: string[]) => string[]) => void,
) {
  const toggleAutoStart = useCallback(
    (langId: string, checked: boolean) => {
      setLspAutoStartLanguages((prev) =>
        checked ? [...prev, langId] : prev.filter((id) => id !== langId),
      );
    },
    [setLspAutoStartLanguages],
  );
  const toggleAutoInstall = useCallback(
    (langId: string, checked: boolean) => {
      setLspAutoInstallLanguages((prev) =>
        checked ? [...prev, langId] : prev.filter((id) => id !== langId),
      );
    },
    [setLspAutoInstallLanguages],
  );
  return { toggleAutoStart, toggleAutoInstall };
}

export function EditorsSettings() {
  const { t } = useTranslation();
  const state = useEditorsSettingsState();
  const {
    setLspAutoStartLanguages,
    setLspAutoInstallLanguages,
    setLspConfigStrings,
    setLspConfigErrors,
    setEditors,
    editors,
  } = state;
  const applyEditors = useApplyEditors(state);
  const saveDefaultRequest = useSaveRequest(state);
  const { createRequest, updateRequest, deleteRequest } = useEditorRequests(state, applyEditors);
  const { updateLspConfigString } = useLspConfigActions(setLspConfigStrings, setLspConfigErrors);
  const isDirty = isEditorsSettingsDirty(state);
  const saveRevision = getEditorsSaveRevision(state);
  const hasInvalidConfig = Object.keys(state.lspConfigErrors).length > 0;

  const { toggleAutoStart, toggleAutoInstall } = useLspLanguageToggles(
    setLspAutoStartLanguages,
    setLspAutoInstallLanguages,
  );

  const customEditors = useMemo(() => sortCustomEditors(editors.filter(isCustomEditor)), [editors]);
  const builtInEditors = useMemo(
    () => editors.filter((editor) => !isCustomEditor(editor)),
    [editors],
  );
  const availableEditors = useMemo(() => resolveAvailableEditors(editors), [editors]);
  const defaultOptions = useMemo<ComboboxOption[]>(
    () => buildDefaultEditorOptions(availableEditors, state.defaultEditorId),
    [availableEditors, state.defaultEditorId],
  );

  useSyncEditors(editors, setEditors);

  return (
    <SettingsPageTemplate
      title={t("settings:editors")}
      description={t("settings:configureTheIncludedCodeEditorAnd")}
      isDirty={isDirty}
      saveStatus={saveDefaultRequest.status}
      saveRevision={saveRevision}
      canSave={!hasInvalidConfig}
      invalidReason={
        hasInvalidConfig ? t("settings:fixInvalidLspServerConfigurationBefore") : undefined
      }
      onSave={() => saveDefaultRequest.run()}
    >
      <div className="space-y-6">
        <div className="space-y-4">
          <div className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
            {t("settings:fileEditor")}
          </div>
          <LspLanguageCards
            lspAutoStartLanguages={state.lspAutoStartLanguages}
            lspAutoInstallLanguages={state.lspAutoInstallLanguages}
            baselineLspAutoStart={state.baselineLspAutoStart}
            baselineLspAutoInstall={state.baselineLspAutoInstall}
            toggleAutoStart={toggleAutoStart}
            toggleAutoInstall={toggleAutoInstall}
          />
          <LspServerConfigSection
            lspConfigStrings={state.lspConfigStrings}
            baselineLspConfigStrings={state.baselineLspConfigStrings}
            lspConfigErrors={state.lspConfigErrors}
            expandedConfigLang={state.expandedConfigLang}
            setExpandedConfigLang={state.setExpandedConfigLang}
            updateLspConfigString={updateLspConfigString}
          />
        </div>
        <Separator />
        <EditorsSection
          defaultOptions={defaultOptions}
          defaultEditorId={state.defaultEditorId}
          baselineDefaultId={state.baselineDefaultId}
          availableEditors={availableEditors}
          builtInEditors={builtInEditors}
          onDefaultEditorChange={state.setDefaultEditorId}
          customEditors={customEditors}
          editingId={state.editingId}
          setEditingId={state.setEditingId}
          isAdding={state.isAdding}
          setIsAdding={state.setIsAdding}
          createRequest={createRequest}
          updateRequest={updateRequest}
          deleteRequest={deleteRequest}
        />
      </div>
    </SettingsPageTemplate>
  );
}

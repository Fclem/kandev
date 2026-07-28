import {
  AgentProfileDeleteConfirmDialog,
  AgentProfileDeleteConflictDialog,
  type AgentProfileDeleteConflict,
} from "@/components/settings/agent-profile-delete-dialog";

type ProfileDeleteDialogsProps = {
  showDeleteConfirm: boolean;
  setShowDeleteConfirm: (open: boolean) => void;
  handleDeleteProfile: () => void;
  conflict: AgentProfileDeleteConflict | null;
  setConflict: (c: AgentProfileDeleteConflict | null) => void;
  handleForceDelete: () => void;
};

export function ProfileDeleteDialogs({
  showDeleteConfirm,
  setShowDeleteConfirm,
  handleDeleteProfile,
  conflict,
  setConflict,
  handleForceDelete,
}: ProfileDeleteDialogsProps) {
  return (
    <>
      <AgentProfileDeleteConfirmDialog
        open={showDeleteConfirm}
        onOpenChange={(open) => {
          if (!open) setShowDeleteConfirm(false);
        }}
        onConfirm={handleDeleteProfile}
      />

      <AgentProfileDeleteConflictDialog
        conflict={conflict}
        onOpenChange={(open) => {
          if (!open) setConflict(null);
        }}
        onConfirm={handleForceDelete}
      />
    </>
  );
}

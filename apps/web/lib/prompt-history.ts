import type { Message, Turn } from "@/lib/types/http";

export type PromptHistoryEntry = {
  messageId: string;
  sessionId: string;
  content: string;
  sentAt: string;
  durationSeconds: number | null;
  isLastPrompt: boolean;
};

export type PromptDurationUnits = {
  s: string;
  m: string;
  h: string;
};

type PromptWithTimestamp = Message & { timestamp: number };

function timestamp(value: string | undefined): number | null {
  if (!value) return null;
  const parsed = Date.parse(value);
  return Number.isNaN(parsed) ? null : parsed;
}

function comparePrompts(a: PromptWithTimestamp, b: PromptWithTimestamp): number {
  return a.timestamp - b.timestamp || a.id.localeCompare(b.id);
}

function turnCompletionByPrompt(messages: PromptWithTimestamp[], turns: Turn[]) {
  const turnsBySessionAndId = new Map<string, Turn>();
  for (const turn of turns) {
    turnsBySessionAndId.set(`${turn.session_id}:${turn.id}`, turn);
  }

  return new Map(
    messages.map((message) => [
      `${message.session_id}:${message.id}`,
      timestamp(turnsBySessionAndId.get(`${message.session_id}:${message.turn_id}`)?.completed_at),
    ]),
  );
}

export function buildPromptHistoryEntries(
  messages: Message[],
  turns: Turn[],
): PromptHistoryEntry[] {
  const prompts = messages
    .flatMap((message) => {
      if (message.author_type !== "user") return [];
      const sentAt = timestamp(message.created_at);
      return sentAt === null ? [] : [{ ...message, timestamp: sentAt }];
    })
    .sort(comparePrompts);
  const completions = turnCompletionByPrompt(prompts, turns);
  const promptsBySession = new Map<string, PromptWithTimestamp[]>();

  for (const prompt of prompts) {
    const sessionPrompts = promptsBySession.get(prompt.session_id) ?? [];
    sessionPrompts.push(prompt);
    promptsBySession.set(prompt.session_id, sessionPrompts);
  }

  const entries = prompts.map((prompt) => {
    const sessionPrompts = promptsBySession.get(prompt.session_id)!;
    const index = sessionPrompts.indexOf(prompt);
    const nextPrompt = sessionPrompts[index + 1];
    const completedAt = completions.get(`${prompt.session_id}:${prompt.id}`) ?? null;
    const nextPromptAt = nextPrompt?.timestamp ?? null;
    const end = Math.min(
      ...[completedAt, nextPromptAt].filter((value): value is number => value !== null),
    );
    const durationSeconds = Number.isFinite(end)
      ? Math.floor(Math.max(0, end - prompt.timestamp) / 1000)
      : null;

    return {
      messageId: prompt.id,
      sessionId: prompt.session_id,
      content: prompt.content,
      sentAt: prompt.created_at,
      durationSeconds,
      isLastPrompt: index === sessionPrompts.length - 1,
    };
  });

  return entries.reverse();
}

export function formatPromptDuration(seconds: number, units: PromptDurationUnits): string {
  const totalSeconds = Math.max(0, Math.floor(seconds));
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const remainingSeconds = totalSeconds % 60;

  if (hours > 0) return `${hours}${units.h} ${minutes}${units.m} ${remainingSeconds}${units.s}`;
  if (minutes > 0) return `${minutes}${units.m} ${remainingSeconds}${units.s}`;
  return `${remainingSeconds}${units.s}`;
}

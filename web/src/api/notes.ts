// Personal sticky notes on the home page (ADR-0052).
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { api, path } from "./client";

export const NOTE_COLORS = ["yellow", "green", "blue", "pink", "purple"] as const;
export type NoteColor = (typeof NOTE_COLORS)[number];

export interface Note {
  id: string;
  body: string;
  color: NoteColor;
  pinned: boolean;
  created_at: string;
  updated_at: string;
}

export type NoteInput = Partial<Pick<Note, "body" | "color" | "pinned">>;

const notesKey = ["me", "notes"] as const;

export const useNotes = () =>
  useQuery({ queryKey: notesKey, queryFn: async () => (await api<{ notes: Note[] }>("GET", "/me/notes")).notes });

export function useNoteApi() {
  const qc = useQueryClient();
  const refresh = () => qc.invalidateQueries({ queryKey: notesKey });
  return {
    create: useMutation({ mutationFn: (body: NoteInput) => api<Note>("POST", "/me/notes", body), onSettled: refresh }),
    update: useMutation({
      mutationFn: (v: { id: string; body: NoteInput }) => api<Note>("PATCH", path`/me/notes/${v.id}`, v.body),
      onSettled: refresh,
    }),
    remove: useMutation({ mutationFn: (id: string) => api<undefined>("DELETE", path`/me/notes/${id}`), onSettled: refresh }),
  };
}

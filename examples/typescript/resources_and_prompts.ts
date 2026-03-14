// examples/typescript/resources_and_prompts.ts
// Demonstrates resources, prompts, and completions — the full MCP feature set.
// Run: pmcp dev examples/typescript/resources_and_prompts.ts

import {
  tool, ToolResult,
  resource, resourceTemplate,
  prompt,
  completion,
} from 'protomcp';
import { z } from 'zod';
import type { ResourceContent, PromptMessage, CompletionResult } from 'protomcp';

// ─── Resources ───────────────────────────────────────────────────────────────

const NOTES: Record<string, string> = {
  'meeting-2024-01-15': 'Discussed Q1 roadmap. Action items: hire 2 engineers.',
  'meeting-2024-01-22': 'Sprint review. Shipped auth system. Demo went well.',
  'meeting-2024-02-01': 'Retrospective. Need better test coverage.',
};

resource({
  uri: 'notes://index',
  description: 'List of all available meeting notes',
  mimeType: 'application/json',
  handler: (uri) => ({
    uri,
    text: JSON.stringify(Object.keys(NOTES)),
    mimeType: 'application/json',
  }),
});

resourceTemplate({
  uriTemplate: 'notes://{note_id}',
  description: 'Read a specific meeting note by ID',
  mimeType: 'text/plain',
  handler: (uri) => {
    const noteId = uri.replace('notes://', '');
    return { uri, text: NOTES[noteId] ?? `Note not found: ${noteId}` };
  },
});

// ─── Prompts ─────────────────────────────────────────────────────────────────

prompt({
  name: 'summarize_note',
  description: 'Summarize a meeting note',
  arguments: [
    { name: 'note_id', description: 'ID of the note to summarize', required: true },
    { name: 'style', description: 'Summary style: brief, detailed, or bullet' },
  ],
  handler: (args) => {
    const text = NOTES[args.note_id] ?? 'Note not found';
    const style = args.style ?? 'brief';
    return [
      { role: 'user', content: `Summarize this meeting note in a ${style} style:\n\n${text}` },
    ];
  },
});

// ─── Completions ─────────────────────────────────────────────────────────────

completion('ref/prompt', 'summarize_note', 'note_id', (value) => {
  const matches = Object.keys(NOTES).filter((id) => id.startsWith(value));
  return { values: matches, total: matches.length, hasMore: false };
});

completion('ref/prompt', 'summarize_note', 'style', (value) => {
  const styles = ['brief', 'detailed', 'bullet'];
  return styles.filter((s) => s.startsWith(value));
});

// ─── Tools ───────────────────────────────────────────────────────────────────

tool({
  name: 'search_notes',
  description: 'Search meeting notes by keyword',
  args: z.object({ query: z.string() }),
  handler: ({ query }) => {
    const matches = Object.entries(NOTES)
      .filter(([, text]) => text.toLowerCase().includes(query.toLowerCase()))
      .reduce((acc, [id, text]) => ({ ...acc, [id]: text }), {});
    return new ToolResult({ result: JSON.stringify(matches) });
  },
});

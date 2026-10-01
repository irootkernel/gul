import {expect, test} from "bun:test";
import {create} from "@bufbuild/protobuf";
import {ConversationEntrySchema, ListConversationResponseSchema, ListPromptHistoryResponseSchema,
  PromptHistoryItemSchema, ListSpecialistResultsResponseSchema, SpecialistResultFormat} from "../../api/generated/ts/gul/v1/gul_pb";
import {maximumPageMetadataBytes, maximumPageSize, maximumPreviewBytes, maximumTokenBytes} from "../../api/generated/ts/gul/v1/bounds";
import {assertConversationPage, assertHistoryPage, assertSpecialistPage} from "./paged-provider-list";

test("conversation page rejects each browser response bound", () => {
  const entry = create(ConversationEntrySchema, {entryId: "entry-1", preview: "ok"});
  const valid = create(ListConversationResponseSchema, {snapshotId: "snapshot", items: [entry], nextPageToken: "next"});
  expect(() => assertConversationPage(valid)).not.toThrow();
  expect(() => assertConversationPage(create(ListConversationResponseSchema,
    {...valid, items: Array.from({length: maximumPageSize}, (_, index) =>
      create(ConversationEntrySchema, {entryId: `entry-${index}`, preview: "x".repeat(maximumPreviewBytes)}))}))).not.toThrow();
  const invalid = [
    {snapshotId: ""},
    {items: Array(maximumPageSize + 1).fill(entry)},
    {nextPageToken: "x".repeat(maximumTokenBytes + 1)},
    {traversalComplete: true},
    {items: [create(ConversationEntrySchema, {entryId: "entry-1", preview: "x".repeat(maximumPreviewBytes + 1)})]},
    {items: [create(ConversationEntrySchema, {entryId: "entry-1", title: "x".repeat(maximumPageMetadataBytes), preview: "ok"})]},
  ];
  for (const fields of invalid) {
    expect(() => assertConversationPage(create(ListConversationResponseSchema, {...valid, ...fields}))).toThrow("Provider page is invalid");
  }
});

test("Prompt History page uses the same bounded admission", () => {
  const item = create(PromptHistoryItemSchema, {promptItemId: "prompt-1", preview: "original"});
  const valid = create(ListPromptHistoryResponseSchema, {snapshotId: "snapshot", items: [item], nextPageToken: "next"});
  expect(() => assertHistoryPage(valid)).not.toThrow();
  expect(() => assertHistoryPage(create(ListPromptHistoryResponseSchema,
    {...valid, items: [create(PromptHistoryItemSchema, {...item, preview: "x".repeat(maximumPreviewBytes + 1)})]}))).toThrow("Provider page is invalid");
});

test("oversized conversation and history pages reject before visiting entries", () => {
  const conversation = create(ListConversationResponseSchema, {
    snapshotId: "snapshot",
    items: Array.from({length: maximumPageSize + 1}, (_, index) =>
      create(ConversationEntrySchema, {entryId: `entry-${index}`, preview: "ok"})),
  });
  const history = create(ListPromptHistoryResponseSchema, {
    snapshotId: "snapshot",
    items: Array.from({length: maximumPageSize + 1}, (_, index) =>
      create(PromptHistoryItemSchema, {promptItemId: `prompt-${index}`, preview: "ok"})),
  });
  for (const page of [conversation, history]) {
    Object.defineProperty(page.items[0], "preview", {get() {
      throw new Error("Entry visited before page count rejection");
    }});
  }
  expect(() => assertConversationPage(conversation)).toThrow("Provider page is invalid");
  expect(() => assertHistoryPage(history)).toThrow("Provider page is invalid");
});

test("result discovery refuses oversized pages and unsupported or unverified artifacts", () => {
  const page = create(ListSpecialistResultsResponseSchema, {snapshotId: "snapshot", items: [{resultId: "result", artifactRef: "artifact", format: SpecialistResultFormat.UTF8_TEXT, sha256: "a".repeat(64)}]});
  expect(() => assertSpecialistPage(page)).not.toThrow();
  expect(() => assertSpecialistPage({...page, items: Array(maximumPageSize + 1).fill(page.items[0])})).toThrow("Provider page is invalid");
  expect(() => assertSpecialistPage({...page, items: [{...page.items[0]!, format: SpecialistResultFormat.UNSPECIFIED}]})).toThrow("Provider result is invalid");
  expect(() => assertSpecialistPage({...page, items: [{...page.items[0]!, sha256: "unverified"}]})).toThrow("Provider result is invalid");
});

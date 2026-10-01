import {toBinary} from "@bufbuild/protobuf";
import {ListConversationResponseSchema, ListPromptHistoryResponseSchema, ListSpecialistResultsResponseSchema, SpecialistResultFormat,
  type ListConversationResponse, type ListPromptHistoryResponse, type ListSpecialistResultsResponse} from "../../api/generated/ts/gul/v1/gul_pb";
import {maximumPageMetadataBytes, maximumPageSize, maximumPreviewBytes, maximumTokenBytes} from "../../api/generated/ts/gul/v1/bounds";

// Keep the provider's page order, refuse a different snapshot, and preserve
// identities already shown when a continuation overlaps its previous page.
export function assertSnapshot(expectedSnapshot: string, actualSnapshot: string) {
  if (actualSnapshot !== expectedSnapshot) throw new Error("Provider snapshot changed");
}

export function appendDistinctPage<T>(current: T[], items: T[], identity: (item: T) => string) {
  const seen = new Set(current.map(identity));
  return [...current, ...items.filter(item => {
    const key = identity(item);
    if (seen.has(key)) return false;
    seen.add(key);
    return true;
  })];
}

const byteLength = (value: string) => new TextEncoder().encode(value).length;

function assertPage(page: ListConversationResponse | ListPromptHistoryResponse | ListSpecialistResultsResponse, encodedLength: () => number) {
  if (!page.snapshotId || page.items.length > maximumPageSize ||
      byteLength(page.nextPageToken ?? "") > maximumTokenBytes ||
      (page.traversalComplete && !!page.nextPageToken) ||
      page.items.some(item => "preview" in item && byteLength(item.preview) > maximumPreviewBytes) ||
      encodedLength() > maximumPageMetadataBytes) {
    throw new Error("Provider page is invalid");
  }
}

export function assertConversationPage(page: ListConversationResponse) {
  assertPage(page, () => toBinary(ListConversationResponseSchema, page).length);
}

export function assertHistoryPage(page: ListPromptHistoryResponse) {
  assertPage(page, () => toBinary(ListPromptHistoryResponseSchema, page).length);
}

export function assertSpecialistPage(page: ListSpecialistResultsResponse) {
  assertPage(page, () => toBinary(ListSpecialistResultsResponseSchema, page).length);
  if (page.items.some(item => !item.resultId || !item.artifactRef || item.format !== SpecialistResultFormat.UTF8_TEXT || !/^[0-9a-f]{64}$/.test(item.sha256))) {
    throw new Error("Provider result is invalid");
  }
}

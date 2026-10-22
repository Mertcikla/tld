import { SmallCloseIcon } from "@chakra-ui/icons";
import { useCallback, useEffect, useRef, useState } from "react";
import {
  Box,
  Button,
  HStack,
  Input,
  IconButton,
  Select,
  Text,
  Textarea,
  VStack,
  useToast,
} from "@chakra-ui/react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { create, fromJson, toJson, type JsonValue } from "@bufbuild/protobuf";
import { ConnectorSchema, ElementSchema } from "@buf/tldiagramcom_diagram.bufbuild_es/diag/v1/workspace_service_pb";
import {
  PlanResourceSchema,
  type PlanChecklistItem,
  type PlanOperationResult,
} from "@buf/tldiagramcom_diagram.bufbuild_es/diag/v1/plan_service_pb";
import {
  plansClient,
  appendPlan,
  observedPlanRevision,
  planProjection,
  resourceLabel,
  resourceId,
  type Plan,
  type PlanProjectedResource,
  type PlanResource,
  type PlanOperationInput,
} from "../api/plans";
import { api, protoConnector, protoElementToLibrary } from "../api/client";
import ElementPanel from "./ElementPanel";
import ConnectorPanel from "./ConnectorPanel";
import {
  MarkdownEditor,
  type MarkdownEditorHandle,
} from "./ViewMarkdownPanel/MarkdownEditor";
import type { Connector, LibraryElement } from "../types";

export default function RepositoryPlans({
  repositoryId,
  onWatch,
}: {
  repositoryId: string;
  onWatch: () => void;
}) {
  const [params, setParams] = useSearchParams();
  const planId = params.get("plan") ?? "";
  const [items, setItems] = useState<Plan[]>([]);
  const [plan, setPlan] = useState<Plan>();
  const [resources, setResources] = useState<PlanProjectedResource[]>([]);
  const [available, setAvailable] = useState<PlanProjectedResource[]>([]);
  const [checklist, setChecklist] = useState<PlanChecklistItem[]>([]);
  const [results, setResults] = useState<PlanOperationResult[]>([]);
  const [selected, setSelected] = useState("");
  const [claim, setClaim] = useState("");
  const [prompt, setPrompt] = useState("");
  const [note, setNote] = useState("");
  const [error, setError] = useState("");
  const [pending, setPending] = useState(0);
  const [review, setReview] = useState(false);
  const timer = useRef<ReturnType<typeof setTimeout>>();
  const pendingSave = useRef(new Map<string, () => Promise<void>>());
  const noteEditor = useRef<MarkdownEditorHandle | null>(null);
  const navigate = useNavigate();
  const toast = useToast();
  const label = (r: PlanResource) => resourceLabel(r, available.flatMap(item => item.resource ? [item.resource] : []));
  const selectedResource = resources.find(
    (v) => v.resource?.key === selected,
  )?.resource;
  const refresh = useCallback(async () => {
    const list = await plansClient.listPlans({ repositoryId });
    setItems(list.plans);
    if (!planId) {
      setPlan(undefined);
      return;
    }
    const [projection, text, claims, all] = await Promise.all([
      planProjection(planId),
      plansClient.getPrompt({ planId }),
      plansClient.getChecklist({ planId }),
      planProjection(planId, true),
    ]);
    setPlan((previous) => {
      if (!projection.plan) return undefined;
      return {
        ...projection.plan,
        name:
          pendingSave.current.has("name") && previous
            ? previous.name
            : projection.plan.name,
        objective:
          pendingSave.current.has("objective") && previous
            ? previous.objective
            : projection.plan.objective,
      };
    });
    setResources(projection.resources);
    if (!pendingSave.current.size) setPrompt(text.text);
    setChecklist(claims.items);
    setResults(projection.operations);
    setAvailable(all.resources);
  }, [repositoryId, planId]);
  useEffect(() => {
    void refresh().catch((e) => setError(String(e)));
  }, [refresh]);
  useEffect(() => {
    let current = true;
    setNote("");
    if (selectedResource?.value.case === "view")
      void plansClient
        .getNote({ planId, viewKey: selectedResource.key })
        .then((r) => {
          if (current) setNote(r.note?.content ?? "");
        })
        .catch(() => undefined);
    return () => {
      current = false;
    };
  }, [planId, selectedResource?.key, selectedResource?.value.case]);
  const run = async (work: () => Promise<unknown>) => {
    setPending((n) => n + 1);
    setError("");
    try {
      await work();
      await refresh();
    } catch (e) {
      setError(String(e));
      throw e;
    } finally {
      setPending((n) => n - 1);
    }
  };
  const saveOperations = (operations: PlanOperationInput[]) =>
    run(() => appendPlan(planId, operations));
  const schedule = (key: string, save: () => Promise<void>) => {
    clearTimeout(timer.current);
    pendingSave.current.set(key, save);
    timer.current = setTimeout(() => {
      void flush().catch(() => undefined);
    }, 400);
  };
  const flush = async () => {
    clearTimeout(timer.current);
    const jobs = [...pendingSave.current.entries()];
    for (const [key, job] of jobs) {
      await job();
      if (pendingSave.current.get(key) === job) pendingSave.current.delete(key);
    }
    if (jobs.length) await refresh();
  };
  useEffect(
    () => () => {
      clearTimeout(timer.current);
      const jobs = [...pendingSave.current.values()];
      pendingSave.current.clear();
      void jobs
        .reduce((p, job) => p.then(job), Promise.resolve())
        .catch(() => undefined);
    },
    [planId],
  );
  const choosePlan = (id: string) =>
    setParams((old) => {
      const next = new URLSearchParams(old);
      if (id) next.set("plan", id);
      else next.delete("plan");
      return next;
    });
  const start = async (resume: boolean) => {
    try {
      const r = await plansClient.createPlan({ repositoryId, resume });
      if (!r.plan) return;
      choosePlan(r.plan.id);
      if (r.resumed)
        toast({
          status: "info",
          title: `Resumed ‘${r.plan.name}’`,
          description: (
            <Button size="xs" onClick={() => void start(false)}>
              Start new instead
            </Button>
          ),
        });
    } catch (e) {
      setError(String(e));
    }
  };
  const metadata = (field: "name" | "objective", value: string) => {
    if (!plan) return;
    setPlan({ ...plan, [field]: value });
    setPrompt((p) =>
      field === "name"
        ? p.replace(plan.name, value)
        : plan.objective
          ? p.replace(plan.objective, value)
          : p.replace("\n\n", "\n\n" + value),
    );
    schedule(field, async () => {
      await run(() =>
        plansClient.updatePlan({
          planId,
          expectedRevision: observedPlanRevision(planId),
          [field]: value,
        }),
      );
    });
  };
  const persistElement = async (data: Partial<LibraryElement>) => {
    if (selectedResource?.value.case !== "element")
      throw new Error("Select an element");
    const json = {
      ...data,
      technology_links: data.technology_connectors,
    } as Record<string, unknown>;
    delete json.technology_connectors;
    const old = selectedResource.value.value;
    const valid = toJson(ElementSchema, old, {
      useProtoFieldName: true,
    }) as Record<string, unknown>;
    const allowed = [
      "name",
      "kind",
      "description",
      "technology",
      "url",
      "logo_url",
      "technology_links",
      "tags",
      "repo",
      "repository_id",
      "branch",
      "file_path",
      "language",
      "bypass_noise_gate",
    ];
    const fields = allowed.filter((k) => json[k] !== undefined);
    for (const k of fields) valid[k] = json[k] ?? "";
    const element = fromJson(ElementSchema, valid as JsonValue);
    const resource = create(PlanResourceSchema, {
      key: selectedResource.key,
      value: { case: "element", value: element },
    });
    await saveOperations([
      {
        action: {
          case: "update",
          value: { resource, fields: { paths: fields } },
        },
      },
    ]);
    return protoElementToLibrary(
      toJson(ElementSchema, element, { useProtoFieldName: true }) as Record<
        string,
        unknown
      >,
    );
  };
  const persistConnector = async (data: Partial<Connector>) => {
    if (!editable || selectedResource?.value.case !== "connector")
      throw new Error("Select an editable connector");
    const valid = toJson(ConnectorSchema, selectedResource.value.value, {
      useProtoFieldName: true,
    }) as Record<string, unknown>;
    const fields = ["label", "description", "relationship", "direction", "style", "url", "tags"]
      .filter((field) => data[field as keyof Connector] !== undefined);
    for (const field of fields) valid[field] = data[field as keyof Connector] ?? "";
    const connector = fromJson(ConnectorSchema, valid as JsonValue);
    await saveOperations([{
      action: {
        case: "update",
        value: {
          resource: create(PlanResourceSchema, {
            key: selectedResource.key,
            value: { case: "connector", value: connector },
          }),
          fields: { paths: fields },
        },
      },
    }]);
    return protoConnector(toJson(ConnectorSchema, connector, {
      useProtoFieldName: true,
    }) as Record<string, unknown>);
  };
  const saveNote = async (content: string) => {
    if (!selectedResource || selectedResource.value.case !== "view") return;
    await saveOperations([
      {
        action: {
          case: "note",
          value: { viewKey: selectedResource.key, content },
        },
      },
    ]);
    await plansClient.getNote({ planId, viewKey: selectedResource.key });
  };
  const editable = plan?.status !== "archived";
  const members = resources.filter(
    (v) => v.inclusionReason === "member" || v.inclusionReason === "changed",
  );
  const viewName = (r: PlanResource) => {
    if (r.value.case === "view") return label(r);
    const v = r.value.value;
    if (v && "viewId" in v)
      return label(
        available.find(
          (x) =>
            x.resource?.value.case === "view" &&
            resourceId(x.resource) === v.viewId,
        )?.resource ?? r,
      );
    const placement = available.find(
      (x) =>
        x.resource?.value.case === "placement" &&
        x.resource.value.value.elementId === resourceId(r),
    );
    if (placement?.resource?.value.case === "placement") {
      const parentId = placement.resource.value.value.viewId;
      return label(
        available.find(
          (x) =>
            x.resource?.value.case === "view" &&
            resourceId(x.resource) === parentId,
        )?.resource ?? r,
      );
    }
    return "Unplaced";
  };
  return (
    <Box p={4} overflowY="auto" w="full" data-testid="repository-plans">
      <HStack mb={4}>
        <Text fontWeight="semibold">Plans</Text>
        <Button size="sm" onClick={() => void start(true)}>
          New
        </Button>
        {plan && (
          <Button size="sm" variant="ghost" onClick={() => choosePlan("")}>
            All plans
          </Button>
        )}
        {plan && available.some((v) => v.resource?.value.case === "view") && (
          <Button
            size="sm"
            onClick={() => {
              const view =
                available.find(
                  (v) =>
                    v.resource?.value.case === "view" &&
                    !v.resource.value.value.ownerElementId,
                )?.resource ??
                available.find((v) => v.resource?.value.case === "view")
                  ?.resource;
              if (view)
                void flush()
                  .then(() =>
                    navigate(`/views/${resourceId(view)}?plan=${planId}`),
                  )
                  .catch((e) => setError(String(e)));
            }}
          >
            Open in editor
          </Button>
        )}
      </HStack>
      {error && (
        <Text role="alert" color="red.300">
          {error}
        </Text>
      )}
      {!plan ? (
        <VStack align="stretch">
          {items.map((p) => (
            <Button
              key={p.id}
              variant="ghost"
              justifyContent="space-between"
              onClick={() => choosePlan(p.id)}
            >
              <Text>{p.name}</Text>
              <Text fontSize="xs">
                {p.status} · {new Date(p.updatedAt).toLocaleString()}
              </Text>
            </Button>
          ))}
        </VStack>
      ) : (
        <Box
          display="grid"
          gridTemplateColumns={{
            base: "1fr",
            lg: "minmax(0,1fr) minmax(0,1fr)",
          }}
          gap={5}
        >
          <VStack align="stretch" minW={0}>
            <Input
              aria-label="Plan name"
              value={plan.name}
              isReadOnly={!editable}
              onChange={(e) => metadata("name", e.target.value)}
            />
            <Textarea
              aria-label="Plan objective"
              placeholder="What should be built?"
              value={plan.objective}
              isReadOnly={!editable}
              onChange={(e) => metadata("objective", e.target.value)}
            />
            {[...new Set(members.map((v) => viewName(v.resource!)))]
              .sort()
              .map((group) => (
                <Box key={group}>
                  <Text fontSize="xs" color="gray.500">
                    {group}
                  </Text>
                  {members
                    .filter((v) => viewName(v.resource!) === group)
                    .map((v) => (
                      <HStack key={v.resource!.key} py={1} minW={0} align="start" flexWrap="wrap">
                        <Button
                          size="sm"
                          minW={0}
                          maxW="full"
                          flex="1 1 180px"
                          height="auto"
                          py={2}
                          whiteSpace="normal"
                          overflowWrap="anywhere"
                          textAlign="left"
                          justifyContent="flex-start"
                          variant={
                            selected === v.resource!.key ? "solid" : "ghost"
                          }
                          onClick={() => {
                            void flush().then(() =>
                              setSelected(v.resource!.key),
                            );
                          }}
                        >
                          {label(v.resource!)}
                        </Button>
                        <Select
                          size="xs"
                          minW="150px" flex="1 1 150px" maxW="220px" aria-label={`Role for ${label(v.resource!)}`}
                          value={v.role || "change"}
                          isDisabled={!editable}
                          onChange={(e) =>
                            void saveOperations([
                              {
                                action: {
                                  case: "include",
                                  value: {
                                    resourceKey: v.resource!.key,
                                    role: e.target.value,
                                    explicit: true,
                                  },
                                },
                              },
                            ]).catch(() => undefined)
                          }
                        >
                          <option value="change">Implement/change</option>
                          <option value="preserve">Preserve</option>
                          <option value="context">Context</option>
                        </Select>
                        <IconButton
                          aria-label={`Remove ${label(v.resource!)} from plan`}
                          title="Remove from plan"
                          icon={<SmallCloseIcon />}
                          size="xs"
                          variant="ghost"
                          color="gray.500"
                          flexShrink={0}
                          isDisabled={!editable || pending > 0}
                          onClick={() =>
                            void flush().then(() => saveOperations([
                              {
                                action: {
                                  case: "exclude",
                                  value: { key: v.resource!.key },
                                },
                              },
                            ])).then(() => {
                              if (selected === v.resource!.key) setSelected("");
                            }).catch(() => undefined)
                          }
                        />
                      </HStack>
                    ))}
                </Box>
              ))}
            {selectedResource?.value.case === "element" && (
              <ElementPanel
                key={selectedResource.key}
                isOpen
                isInline
                hasBackdrop={false}
                autoSave
                element={protoElementToLibrary(
                  toJson(ElementSchema, selectedResource.value.value, {
                    useProtoFieldName: true,
                  }) as Record<string, unknown>,
                )}
                persistElement={persistElement}
                onClose={() => setSelected("")}
                onSave={() => undefined}
              />
            )}
            {selectedResource?.value.case === "connector" && (
              <ConnectorPanel
                key={selectedResource.key}
                isOpen
                isInline
                hasBackdrop={false}
                autoSave
                connector={protoConnector(toJson(
                  ConnectorSchema,
                  selectedResource.value.value,
                  { useProtoFieldName: true },
                ) as Record<string, unknown>)}
                orgId=""
                persistConnector={persistConnector}
                onClose={() => setSelected("")}
                onSave={() => undefined}
              />
            )}
            {selectedResource?.value.case === "view" && (
              <>
                <HStack>
                  <Button
                    size="sm"
                    onClick={() =>
                      navigate(
                        `/views/${resourceId(selectedResource)}?plan=${planId}`,
                      )
                    }
                  >
                    Open in editor
                  </Button>
                  <Button
                    size="sm"
                    onClick={() => {
                      const ids = new Set(
                        available
                          .filter(
                            (v) =>
                              v.resource?.value.case === "placement" &&
                              v.resource.value.value.viewId ===
                                resourceId(selectedResource),
                          )
                          .map(
                            (v) =>
                              v.resource!.value.value! as { elementId: number },
                          )
                          .map((v) => v.elementId),
                      );
                      const included = available.filter(
                        (v) =>
                          (v.resource?.value.case === "element" &&
                            ids.has(resourceId(v.resource))) ||
                          (v.resource?.value.value &&
                            "viewId" in v.resource.value.value &&
                            v.resource.value.value.viewId ===
                              resourceId(selectedResource)),
                      );
                      void saveOperations(
                        included.map((v) => ({
                          action: {
                            case: "include" as const,
                            value: {
                              resourceKey: v.resource!.key,
                              role: "change",
                              explicit: true,
                            },
                          },
                        })),
                      ).catch(() => undefined);
                    }}
                  >
                    Include contents
                  </Button>
                </HStack>
                <Box minH="300px">
                  <MarkdownEditor
                    canEditDocument={editable}
                    content={note}
                    currentMermaidBlock={null}
                    editorRef={noteEditor}
                    handleSyncMermaidBlock={() => undefined}
                    isDirty={pendingSave.current.size > 0}
                    isLoading={false}
                    isSaving={pending > 0}
                    markdown={null}
                    mermaidIntegrationEnabled={false}
                    mermaidContextValue={{
                      blockStatusByCode: new Map(),
                      canEdit: editable,
                      currentViewId: resourceId(selectedResource),
                    }}
                    mermaidSyncStatus={null}
                    onChange={(value) => {
                      setNote(value);
                      setPrompt((p) =>
                        p.includes("Note edits pending")
                          ? p
                          : p + "\nNote edits pending save.",
                      );
                      schedule("note:" + selectedResource.key, () =>
                        saveNote(value),
                      );
                    }}
                    onClose={() => setSelected("")}
                    onSave={saveNote}
                    viewId={resourceId(selectedResource)}
                  />
                </Box>
              </>
            )}
            <Text fontWeight="semibold">Checklist</Text>
            {checklist.map((item) => (
              <HStack key={item.id}>
                <Input
                  aria-label="Requirement text"
                  defaultValue={item.text}
                  isReadOnly={!editable}
                  onBlur={(e) => {
                    if (e.target.value !== item.text)
                      void saveOperations([
                        {
                          action: {
                            case: "checklist",
                            value: { ...item, text: e.target.value },
                          },
                        },
                      ]).catch(() => undefined);
                  }}
                />
                <Button
                  size="xs"
                  onClick={() =>
                    void saveOperations([
                      {
                        action: {
                          case: "removeChecklist",
                          value: { key: item.id },
                        },
                      },
                    ]).catch(() => undefined)
                  }
                >
                  Remove
                </Button>
              </HStack>
            ))}
            <HStack>
              <Input
                aria-label="New requirement"
                value={claim}
                onChange={(e) => setClaim(e.target.value)}
                placeholder="Requirement or constraint"
              />
              <Button
                isDisabled={!claim.trim() || !editable}
                onClick={() =>
                  void saveOperations([
                    {
                      action: {
                        case: "checklist",
                        value: {
                          text: claim,
                          verificationKind: "manual",
                          resourceKeys: selected ? [selected] : [],
                        },
                      },
                    },
                  ])
                    .then(() => setClaim(""))
                    .catch(() => undefined)
                }
              >
                Add
              </Button>
            </HStack>
          </VStack>
          <VStack align="stretch" minW={0}>
            <HStack>
              <Text fontWeight="semibold" flex={1}>
                Agent prompt
              </Text>
              <Text fontSize="xs">{pending ? "Saving…" : plan.status}</Text>
              <Button
                size="sm"
                isDisabled={pending > 0 || !!error}
                onClick={() =>
                  void flush()
                    .then(async () => {
                      const fresh = await plansClient.getPrompt({ planId });
                      await navigator.clipboard.writeText(fresh.text);
                      toast({ title: "Prompt copied", status: "success" });
                    })
                    .catch((e) => setError(String(e)))
                }
              >
                Copy
              </Button>
            </HStack>
            <Textarea
              aria-label="Plan prompt"
              value={prompt}
              isReadOnly
              minH="420px"
              fontFamily="mono"
              fontSize="sm"
            />
            <HStack>
              <Button size="sm" onClick={() => setReview((v) => !v)}>
                Review operations
              </Button>
              <Button
                size="sm"
                isDisabled={pending > 0 || !!error || !editable}
                onClick={() =>
                  void flush()
                    .then(() =>
                      run(async () => {
                        return plansClient.updatePlan({
                          planId,
                          expectedRevision: observedPlanRevision(planId),
                          status: "ready",
                        });
                      }),
                    )
                    .catch(() => undefined)
                }
              >
                Mark ready
              </Button>
              <Box flex={1} />
              <Button size="sm" isLoading={pending > 0} isDisabled={!editable} onClick={() => void flush().then(() => run(async () => {
                const status = await api.repositories.watchStatus(repositoryId);
                await plansClient.lockImplementation({ planId, expectedRevision: observedPlanRevision(planId), gitRevision: status.gitRevision || '' });
                if (!status.running) await api.repositories.startWatch(repositoryId, { materialize: false });
              })).then(onWatch).catch((e) => setError(`Workspace remains locked if watching failed. Retry to start watching. ${String(e)}`))}>
                Lock and start watch
              </Button>
            </HStack>
            {review &&
              results.map((result) => (
                <Box key={result.operationId} p={2} borderWidth="1px">
                  <Text fontSize="sm">
                    {result.status}: {result.reason || result.operationId}
                  </Text>

                </Box>
              ))}
          </VStack>
        </Box>
      )}
    </Box>
  );
}

import { SmallCloseIcon } from "@chakra-ui/icons";
import { useCallback, useEffect, useRef, useState, useSyncExternalStore } from "react";
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
  updatePlan,
  startPlanImplementation,
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
import { planSaveQueue } from "../utils/planSaveQueue";
import { flushPlanEdits } from "../utils/planSaveParticipants";
import { usePlanImplementation } from "../hooks/usePlanImplementation";
import PlanImplementationReview from "./PlanImplementationReview";
import PlanResourceDiff from "./PlanResourceDiff";
import ConnectorPanel from "./ConnectorPanel";
import {
  MarkdownEditor,
  type MarkdownEditorHandle,
} from "./ViewMarkdownPanel/MarkdownEditor";
import type { Connector, LibraryElement } from "../types";
import { ViewEditorContext, useViewEditorContext } from "../pages/ViewEditor/context";

export default function RepositoryPlans(props: { repositoryId: string; onWatch: () => void; compact?: boolean; planId?: string; onPlanChange?: (id: string) => void }) {
  const [params] = useSearchParams();
  const planId = props.planId ?? params.get("plan") ?? "";
  return <RepositoryPlanSession key={props.repositoryId + ":" + planId} {...props} planId={planId} />;
}

function RepositoryPlanSession({ repositoryId, onWatch, compact = false, planId, onPlanChange }: {
  repositoryId: string; onWatch: () => void; compact?: boolean; planId: string; onPlanChange?: (id: string) => void;
}) {
  const [, setParams] = useSearchParams();
  const panelContext = useViewEditorContext();
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
  const pendingSave = useRef(planSaveQueue(planId));
  useSyncExternalStore(pendingSave.current.subscribe, pendingSave.current.snapshot);
  const loadGeneration = useRef(0);
  const mounted = useRef(true);
  const [search, setSearch] = useState("");
  const [addRole, setAddRole] = useState("change");
  const [watchError, setWatchError] = useState("");
  const [watchEnabled, setWatchEnabled] = useState(false);
  const implementation = usePlanImplementation("", "", true);
  const locked = implementation.state?.locked ?? false;
  const implementing = locked && implementation.state?.planId === planId;
  const noteEditor = useRef<MarkdownEditorHandle | null>(null);
  const navigate = useNavigate();
  const toast = useToast();
  const label = (r: PlanResource) => resourceLabel(r, available.flatMap(item => item.resource ? [item.resource] : []));
  const selectedResource = resources.find(
    (v) => v.resource?.key === selected,
  )?.resource;
  const refresh = useCallback(async () => {
    const generation = ++loadGeneration.current;
    const list = await plansClient.listPlans({ repositoryId });
    if (!mounted.current || generation !== loadGeneration.current) return;
    setItems(list.plans);
    if (!planId) { setPlan(undefined); return; }
    const projection = await planProjection(planId, true);
    if (projection.plan?.repositoryId !== repositoryId) {
      throw new Error("This plan belongs to a different repository. Select a plan for this repository.");
    }
    const [text, claims] = await Promise.all([
      plansClient.getPrompt({ planId, revision: projection.revision }),
      plansClient.getChecklist({ planId, revision: projection.revision }),
    ]);
    if (!mounted.current || generation !== loadGeneration.current) return;
    setPlan(projection.plan && { ...projection.plan,
      name: pendingSave.current.draft("name") ?? projection.plan.name,
      objective: pendingSave.current.draft("objective") ?? projection.plan.objective,
    });
    setResources(projection.resources.filter(v => v.inclusionReason === "member" || v.inclusionReason === "changed"));
    setPrompt(text.text);
    setChecklist(claims.items);
    setResults(projection.operations);
    setAvailable(projection.resources);
  }, [repositoryId, planId]);
  useEffect(() => {
    mounted.current = true;
    void refresh().catch(e => { if (mounted.current) setError(String(e)); });
    const changed = (event: Event) => {
      if ((event as CustomEvent<string>).detail === planId) void refresh().catch(e => { if (mounted.current) setError(String(e)); });
    };
    window.addEventListener("tld:plan-changed", changed);
    return () => { mounted.current = false; window.removeEventListener("tld:plan-changed", changed); };
  }, [refresh, planId]);
  useEffect(() => {
    let active = true;
    void api.system.capabilities().then(caps => { if (active) setWatchEnabled(caps.watch); }).catch(() => undefined);
    return () => { active = false; };
  }, []);
  useEffect(() => {
    let current = true;
    setNote("");
    if (selectedResource?.value.case === "view")
      void plansClient
        .getNote({ planId, viewKey: selectedResource.key })
        .then((r) => {
          if (current) setNote(pendingSave.current.draft("note:" + selectedResource.key) ?? r.note?.content ?? "");
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
      if (mounted.current) await refresh();
    } catch (e) {
      if (mounted.current) setError(String(e));
      throw e;
    } finally {
      setPending((n) => n - 1);
    }
  };
  const saveOperations = (operations: PlanOperationInput[]) =>
    run(() => appendPlan(planId, operations));
  const schedule = (key: string, save: () => Promise<void>, value?: string) => {
    clearTimeout(timer.current);
    pendingSave.current.set(key, save, value);
    timer.current = setTimeout(() => { void flush().catch(() => undefined); }, 400);
  };
  const flush = async () => {
    clearTimeout(timer.current);
    await flushPlanEdits(planId);
    if (mounted.current) { setError(""); await refresh(); }
  };
  useEffect(() => {
    const queue = pendingSave.current;
    const beforeUnload = (event: BeforeUnloadEvent) => {
      if (queue.size) { event.preventDefault(); event.returnValue = ""; }
    };
    window.addEventListener("beforeunload", beforeUnload);
    return () => {
      window.removeEventListener("beforeunload", beforeUnload);
      clearTimeout(timer.current);
      // The queue retains failed input when the user returns to this plan.
      void queue.flush().catch(() => undefined);
    };
  }, []);
  const choosePlan = (id: string) => {
    if (onPlanChange) { onPlanChange(id); return; }
    setParams((old) => {
      const next = new URLSearchParams(old);
      if (id) next.set("plan", id);
      else next.delete("plan");
      return next;
    });
  };
  const start = async (resume: boolean) => {
    if (pending > 0) return;
    setPending(n => n + 1);
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
    } finally { if (mounted.current) setPending(n => n - 1); }
  };
  const metadata = (field: "name" | "objective", value: string) => {
    if (!plan) return;
    setPlan({ ...plan, [field]: value });
    schedule(field, async () => { await updatePlan(planId, { [field]: value }); }, value);
  };

  const persistElement = async (data: Partial<LibraryElement>) => {
    if (!editable || selectedResource?.value.case !== "element")
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
  const ensureWatch = async () => {
    const status = await api.repositories.watchStatus(repositoryId);
    if (!status.running) await api.repositories.startWatch(repositoryId, { materialize: false });
  };
  const editable = !!plan && !["archived", "completed"].includes(plan.status) && !locked;
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
        <Button size="sm" isDisabled={pending > 0} onClick={() => void flush().then(() => start(false)).catch(e => setError(String(e)))}>
          New plan
        </Button>
        {plan && (
          <Button size="sm" variant="ghost" onClick={() => void flush().then(() => choosePlan("")).catch(e => setError(String(e)))}>
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
      <PlanImplementationReview implementation={implementation} />
      {pendingSave.current.error && <Box role="alert" mb={3}>
        <Text>{pendingSave.current.error}</Text>
        <Button size="sm" onClick={() => void flush().catch(e => setError(String(e)))}>Retry saving</Button>
      </Box>}
      {plan && <Box mb={4}>
        <Text fontSize="sm" fontWeight="semibold">{plan.status === "completed" ? "Implementation finished" : implementing ? "Implement → Review → Finish" : "Describe → Design → Start implementation"}</Text>
        <Text fontSize="sm" color="gray.500">{plan.status === "completed" ? "Diagram updates have been reviewed and the workspace is released." : implementing
          ? "The specification is frozen. Workspace edits are staged for review."
          : "You’re editing the target architecture. Diagram changes save immediately; code has not been implemented yet."}</Text>
      </Box>}
      {!plan ? (
        <VStack align="stretch">
          {!items.length && <Text>Create a plan, describe the goal, then add diagram objects to its scope.</Text>}
          {items.map((p) => (
            <Button
              key={p.id}
              variant="ghost"
              justifyContent="space-between"
              onClick={() => void flush().then(() => choosePlan(p.id)).catch(e => setError(String(e)))}
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
            lg: compact ? "1fr" : "minmax(0,1fr) minmax(0,1fr)",
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
            <Text fontWeight="semibold">Scope · {members.length} items</Text>
            {!members.length && <Text fontSize="sm" color="gray.500">Search for an object below, or open the editor and add your selection.</Text>}
            {editable && <VStack align="stretch">
              <Input aria-label="Search plan scope" placeholder="Find diagram objects to add…" value={search} onChange={e => setSearch(e.target.value)} />
              {search.trim() && <>
                <Select aria-label="Role for new scope items" size="sm" value={addRole} onChange={e => setAddRole(e.target.value)}>
                  <option value="change">Change</option><option value="preserve">Keep unchanged</option><option value="context">Reference</option>
                </Select>
                {available.filter(v => v.resource && ["element", "connector", "view"].includes(v.resource.value.case ?? "") && !members.some(m => m.resource?.key === v.resource?.key) && label(v.resource).toLowerCase().includes(search.toLowerCase())).slice(0, 20).map(v => <Button size="sm" key={v.resource!.key} whiteSpace="normal" height="auto" py={2}
                  onClick={() => void saveOperations([{ action: { case: "include", value: { resourceKey: v.resource!.key, role: addRole, explicit: true } } }]).catch(() => undefined)}>
                  Add {label(v.resource!)}
                </Button>)}
              </>}
            </VStack>}
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
                          <option value="change">Change</option>
                          <option value="preserve">Keep unchanged</option>
                          <option value="context">Reference</option>
                        </Select>
                        <IconButton
                          aria-label={`Remove ${label(v.resource!)} from plan`}
                          title="Remove from scope; diagram changes remain saved"
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
              <ViewEditorContext.Provider value={{ ...panelContext, canEdit: editable }}><Box as="fieldset" disabled={!editable} border={0} p={0} minW={0}><ElementPanel
                key={selectedResource.key}
                isOpen
                isInline
                hasBackdrop={false}
                autoSave
                planId={planId}
                element={protoElementToLibrary(
                  toJson(ElementSchema, selectedResource.value.value, {
                    useProtoFieldName: true,
                  }) as Record<string, unknown>,
                )}
                persistElement={persistElement}
                onClose={() => setSelected("")}
                onSave={() => undefined}
              /></Box></ViewEditorContext.Provider>
            )}
            {selectedResource?.value.case === "connector" && (
              <ViewEditorContext.Provider value={{ ...panelContext, canEdit: editable }}><Box as="fieldset" disabled={!editable} border={0} p={0} minW={0}><ConnectorPanel
                key={selectedResource.key}
                isOpen
                isInline
                hasBackdrop={false}
                autoSave
                planId={planId}
                connector={protoConnector(toJson(
                  ConnectorSchema,
                  selectedResource.value.value,
                  { useProtoFieldName: true },
                ) as Record<string, unknown>)}
                orgId=""
                persistConnector={persistConnector}
                onClose={() => setSelected("")}
                onSave={() => undefined}
              /></Box></ViewEditorContext.Provider>
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
                    isDisabled={!editable}
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
                        saveNote(value), value,
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
                  isDisabled={!editable}
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
                Implementation prompt
              </Text>
              <Text fontSize="xs" role="status">{pendingSave.current.error ? "Save failed" : pendingSave.current.size || pending ? "Saving…" : "Saved"}</Text>
              <Button
                size="sm"
                isDisabled={pending > 0 || !implementing}
                onClick={() =>
                  void flush()
                    .then(async () => {
                      const fresh = await plansClient.getPrompt({ planId, revision: plan.readyRevision || plan.revision });
                      await navigator.clipboard.writeText(fresh.text);
                      toast({ title: "Prompt copied", status: "success" });
                    })
                    .catch((e) => setError(String(e)))
                }
              >
                Copy prompt
              </Button>
            </HStack>
            {!implementing && <Text fontSize="sm" color="gray.500">Start implementation to freeze this specification and copy the agent prompt.</Text>}
            <Textarea
              aria-label="Plan prompt"
              value={prompt}
              isReadOnly
              minH={compact ? "160px" : "260px"}
              fontFamily="mono"
              fontSize="sm"
            />
            <HStack flexWrap="wrap">
              <Button size="sm" onClick={() => setReview(v => !v)}>Review diagram changes</Button>
              {plan.status === "completed" && <Button size="sm" onClick={() => void run(() => updatePlan(planId, { status: "draft" })).catch(() => undefined)}>Reopen as draft</Button>}
              {!implementing && plan.status !== "completed" && <Button size="sm" colorScheme="blue" isLoading={pending > 0}
                isDisabled={!editable || !plan.objective.trim() || !members.length}
                onClick={() => void flush().then(() => run(async () => {
                  await startPlanImplementation(planId);
                  setWatchError("");
                  if (watchEnabled) {
                    try { await ensureWatch(); }
                    catch (e) { setWatchError(`Implementation prepared; Watch unavailable. ${String(e)}`); }
                  }
                })).catch(e => setError(String(e)))}>
                Start implementation
              </Button>}
              {implementing && watchEnabled && <Button size="sm" onClick={onWatch}>Open Watch</Button>}
            </HStack>
            {editable && (!plan.objective.trim() || !members.length) && <Text fontSize="sm" color="gray.500">
              Add an objective and at least one scope item to start implementation.
            </Text>}
            {watchError && <Box role="alert"><Text fontSize="sm">{watchError}</Text>
              <Button size="sm" onClick={() => void ensureWatch().then(() => setWatchError("")).catch(e => setWatchError(String(e)))}>Retry Watch</Button>
            </Box>}
            {review && <VStack align="stretch">
              <Text fontSize="xs" color="gray.500">Changes since each object first entered this plan. Removing an item from scope does not undo its saved diagram changes.</Text>
              {resources.filter(item => item.change).map(item => <Box key={item.resource!.key} p={2} borderWidth="1px" borderRadius="md">
                <Text fontSize="sm">{item.change}: {label(item.resource!)}</Text>
                <PlanResourceDiff before={item.before} after={item.change === "remove" ? undefined : item.resource} />
              </Box>)}
              {!resources.some(item => item.change) && <Text fontSize="sm">No diagram changes recorded. Scope and requirements describe the requested work.</Text>}
              {!!results.filter(result => result.reason).length && <Text fontSize="sm">{results.filter(result => result.reason).map(result => result.reason).join(" · ")}</Text>}
            </VStack>}
          </VStack>
        </Box>
      )}
    </Box>
  );
}

import { useEffect, useState } from "react";
import {
  Button,
  HStack,
  Select,
  Popover,
  PopoverTrigger,
  PopoverContent,
  PopoverBody,
  useToast,
} from "@chakra-ui/react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { api, type IndexedRepository } from "../api/client";
import {
  appendPlan,
  plansClient,
  planProjection,
  resourceId,
  type Plan,
} from "../api/plans";
import RepositoryTargetPicker from "./RepositoryTargetPicker";

export default function PlanControls({
  viewId,
  elementIds = [],
  connectorIds = [],
}: {
  viewId: number | null;
  elementIds?: number[];
  connectorIds?: number[];
}) {
  const [params] = useSearchParams();
  const id = params.get("plan") ?? "";
  const [plan, setPlan] = useState<Plan>();
  const [repositories, setRepositories] = useState<IndexedRepository[]>([]);
  const [repository, setRepository] = useState(params.get("repo") ?? "");
  const [picking, setPicking] = useState(false);
  const [role, setRole] = useState("change");
  const toast = useToast();
  const navigate = useNavigate();
  useEffect(() => {
    if (!id) {
      setPlan(undefined);
      return;
    }
    void plansClient
      .getPlan({ planId: id })
      .then((r) => {
        setPlan(r.plan);
        if (r.plan && sessionStorage.getItem("tld:resumed-plan") === id) {
          sessionStorage.removeItem("tld:resumed-plan");
          const repositoryId = r.plan.repositoryId;
          toast({
            title: `Resumed ‘${r.plan.name}’`,
            description: (
              <Button
                size="xs"
                onClick={() => {
                  void plansClient
                    .createPlan({ repositoryId, resume: false })
                    .then((result) => {
                      if (result.plan)
                        window.location.assign(
                          `${window.location.pathname}?plan=${encodeURIComponent(result.plan.id)}&repo=${encodeURIComponent(repositoryId)}`,
                        );
                    })
                    .catch((e) => toast({ title: String(e), status: "error" }));
                }}
              >
                Start new instead
              </Button>
            ),
          });
        }
      })
      .catch((e) => toast({ title: String(e), status: "error" }));
  }, [id, toast]);
  const start = async (resume: boolean) => {
    try {
      let repo = repository;
      if (!repo) {
        const list = await api.repositories.list();
        setRepositories(list);
        if (list.length === 1) {
          repo = list[0].id;
          setRepository(repo);
        } else {
          setPicking(true);
          return;
        }
      }
      const response = await plansClient.createPlan({
        repositoryId: repo,
        resume,
      });
      if (!response.plan) return;
      if (response.resumed)
        sessionStorage.setItem("tld:resumed-plan", response.plan.id);
      // Remount to refresh the active plan context.
      window.location.assign(
        `${window.location.pathname}?plan=${encodeURIComponent(response.plan.id)}&repo=${encodeURIComponent(repo)}`,
      );
    } catch (e) {
      toast({ title: String(e), status: "error" });
    }
  };
  const include = async () => {
    try {
      const projection = await planProjection(id, true);
      const matches = projection.resources.filter((v) => {
        const r = v.resource;
        if (!r) return false;
        return (
          (r.value.case === "element" && elementIds.includes(resourceId(r))) ||
          (r.value.case === "connector" &&
            connectorIds.includes(resourceId(r))) ||
          (!elementIds.length &&
            !connectorIds.length &&
            r.value.case === "view" &&
            resourceId(r) === viewId)
        );
      });
      if (!matches.length) return;
      await appendPlan(
        id,
        matches.map((v) => ({
          action: {
            case: "include",
            value: { resourceKey: v.resource!.key, role, explicit: true },
          },
        })),
      );
      window.location.reload();
    } catch (e) {
      toast({ title: String(e), status: "error" });
    }
  };
  if (id)
    return (
      <HStack flexWrap="wrap" justify="center" gap={1} minW={0}>
        <Popover>
          <PopoverTrigger>
            <Button size="xs" variant="ghost" maxW="220px" overflow="hidden" textOverflow="ellipsis" display="block">
              Planning · {plan?.name ?? "Loading…"}
            </Button>
          </PopoverTrigger>
          <PopoverContent w="auto">
            <PopoverBody>
              <HStack>
                <Select
                  size="xs"
                  w="125px"
                  aria-label="Plan role"
                  value={role}
                  onChange={(e) => setRole(e.target.value)}
                >
                  <option value="change">Implement/change</option>
                  <option value="preserve">Preserve</option>
                  <option value="context">Context</option>
                </Select>
                <Button size="xs" onClick={() => void include()}>
                  Include{" "}
                  {elementIds.length || connectorIds.length
                    ? "selection"
                    : "view"}
                </Button>
              </HStack>
            </PopoverBody>
          </PopoverContent>
        </Popover>
        <Button
          size="xs"
          onClick={() =>
            navigate(
              `/repositories?repo=${plan?.repositoryId ?? repository}&mode=plans&plan=${id}`,
            )
          }
        >
          Review plan
        </Button>
        <Button
          size="xs"
          onClick={() => window.location.assign(window.location.pathname)}
        >
          Exit plan
        </Button>
      </HStack>
    );
  return (
    <HStack>
      {picking && (
        <RepositoryTargetPicker
          value={repository}
          groups={[
            {
              options: repositories.map((r) => ({
                value: r.id,
                label: r.name || r.root,
              })),
            },
          ]}
          onChange={setRepository}
          aria-label="Plan repository"
        />
      )}
      <Button size="xs" onClick={() => void start(true)}>
        New plan
      </Button>
    </HStack>
  );
}

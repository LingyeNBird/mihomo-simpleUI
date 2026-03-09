import {
  CheckCircleOutlined,
  CloseCircleOutlined,
  InfoCircleOutlined,
  PlusOutlined,
  ReloadOutlined,
  SyncOutlined,
} from "@ant-design/icons";
import {
  App as AntApp,
  Alert,
  Button,
  Card,
  Col,
  ConfigProvider,
  Drawer,
  Empty,
  Flex,
  Form,
  Input,
  Layout,
  List,
  message,
  Popconfirm,
  Row,
  Segmented,
  Space,
  Spin,
  Statistic,
  Switch,
  Tag,
  Tooltip,
  Typography,
} from "antd";
import { useCallback, useEffect, useMemo, useState } from "react";
import { api } from "./api";
import type { AppStatus, ProxyGroup, Subscription, SubscriptionContent } from "./types";

const { Content } = Layout;
const { Title, Paragraph, Text } = Typography;

interface SubscriptionFormValues {
  name: string;
  url: string;
  enabled: boolean;
}

function formatDate(value?: string) {
  if (!value) return "-";
  return new Date(value).toLocaleString();
}

function uniqueItems(items: string[]) {
  return Array.from(new Set(items));
}

function formatNodeSourceLabel(nodeName: string, sources?: string[]) {
  if (nodeName === "DIRECT" || nodeName === "REJECT") {
    return "来源：内置策略";
  }
  if (nodeName === "Auto") {
    return "来源：自动测速组";
  }
  if (!sources || sources.length === 0) {
    return "来源：未知";
  }
  return sources.length === 1 ? `来源：${sources[0]}` : `来源：${sources.join(" / ")}`;
}

function sortProxyGroups(groups: ProxyGroup[]) {
  const priority = new Map<string, number>([
    ["Proxy", 0],
    ["Auto", 1],
  ]);

  return [...groups].sort((left, right) => {
    const leftPriority = priority.get(left.name) ?? 10;
    const rightPriority = priority.get(right.name) ?? 10;
    if (leftPriority !== rightPriority) {
      return leftPriority - rightPriority;
    }
    return left.name.localeCompare(right.name, "zh-CN");
  });
}

function AppInner() {
  const [messageApi, contextHolder] = message.useMessage();
  const [form] = Form.useForm<SubscriptionFormValues>();
  const [status, setStatus] = useState<AppStatus | null>(null);
  const [subscriptions, setSubscriptions] = useState<Subscription[]>([]);
  const [proxyGroups, setProxyGroups] = useState<ProxyGroup[]>([]);
  const [proxyGroupError, setProxyGroupError] = useState<string>("");
  const [viewerOpen, setViewerOpen] = useState(false);
  const [viewerLoading, setViewerLoading] = useState(false);
  const [viewerData, setViewerData] = useState<SubscriptionContent | null>(null);
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [editing, setEditing] = useState<Subscription | null>(null);
  const [subscriptionPanelOpen, setSubscriptionPanelOpen] = useState(false);
  const [syncing, setSyncing] = useState(false);
  const [selectingGroup, setSelectingGroup] = useState("");
  const [activeProxyGroupName, setActiveProxyGroupName] = useState("");

  const refreshAll = useCallback(async () => {
    setLoading(true);
    try {
      const [nextStatus, nextSubscriptions, nextGroups] = await Promise.all([
        api.status(),
        api.listSubscriptions(),
        api.proxyGroups().catch((error: Error) => {
          setProxyGroupError(error.message);
          return [];
        }),
      ]);
      setStatus(nextStatus);
      setSubscriptions(nextSubscriptions);
      setProxyGroups(nextGroups);
      if (nextGroups.length > 0) {
        setProxyGroupError("");
      }
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : "加载失败");
    } finally {
      setLoading(false);
    }
  }, [messageApi]);

  useEffect(() => {
    void refreshAll();
  }, [refreshAll]);

  const onFinish = async (values: SubscriptionFormValues) => {
    setSubmitting(true);
    try {
      if (editing) {
        await api.updateSubscription(editing.id, values);
        messageApi.success("订阅已更新");
      } else {
        const result = await api.createSubscription(values);
        if (result.warnings?.length) {
          messageApi.warning(result.warnings.join("；"));
        } else {
          messageApi.success("订阅已添加并拉取");
        }
      }
      form.resetFields();
      setEditing(null);
      setSubscriptionPanelOpen(false);
      await refreshAll();
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : "保存失败");
    } finally {
      setSubmitting(false);
    }
  };

  const visibleProxyGroups = useMemo(
    () => sortProxyGroups(proxyGroups.filter((group) => group.name !== "GLOBAL")),
    [proxyGroups],
  );

  const hiddenGlobalGroup = useMemo(
    () => proxyGroups.find((group) => group.name === "GLOBAL"),
    [proxyGroups],
  );

  const activeProxyGroup = useMemo(() => {
    if (visibleProxyGroups.length === 0) {
      return null;
    }
    return visibleProxyGroups.find((group) => group.name === activeProxyGroupName) ?? visibleProxyGroups[0];
  }, [activeProxyGroupName, visibleProxyGroups]);

  useEffect(() => {
    if (visibleProxyGroups.length === 0) {
      if (activeProxyGroupName !== "") {
        setActiveProxyGroupName("");
      }
      return;
    }
    if (!visibleProxyGroups.some((group) => group.name === activeProxyGroupName)) {
      setActiveProxyGroupName(visibleProxyGroups[0].name);
    }
  }, [activeProxyGroupName, visibleProxyGroups]);

  const selectProxyNode = useCallback(
    async (groupName: string, nodeName: string) => {
      setSelectingGroup(groupName);
      try {
        await api.selectNode(groupName, nodeName);
        messageApi.success(`已将 ${groupName} 切换到 ${nodeName}`);
        await refreshAll();
      } catch (error) {
        messageApi.error(error instanceof Error ? error.message : "切换失败");
      } finally {
        setSelectingGroup("");
      }
    },
    [messageApi, refreshAll],
  );

  const closeSubscriptionPanel = useCallback(() => {
    setSubscriptionPanelOpen(false);
    setEditing(null);
    form.resetFields();
  }, [form]);

	const openCreateSubscription = useCallback(() => {
		form.resetFields();
		setEditing(null);
		setSubscriptionPanelOpen(true);
	}, [form]);

	const openSubscriptionViewer = useCallback(
		async (item: Subscription) => {
			try {
				setViewerLoading(true);
				const content = await api.subscriptionContent(item.id);
				setViewerData(content);
				setViewerOpen(true);
			} catch (error) {
				messageApi.error(error instanceof Error ? error.message : "读取订阅内容失败");
			} finally {
				setViewerLoading(false);
			}
		},
		[messageApi],
	);

	const editSubscription = useCallback(
		(item: Subscription) => {
			setEditing(item);
			form.setFieldsValue({ name: item.name, url: item.url, enabled: item.enabled });
			setSubscriptionPanelOpen(true);
		},
		[form],
	);

	const refreshSubscriptionItem = useCallback(
		async (item: Subscription) => {
			try {
				await api.refreshSubscription(item.id);
				messageApi.success(`已刷新 ${item.name}`);
				await refreshAll();
			} catch (error) {
				messageApi.error(error instanceof Error ? error.message : "刷新失败");
			}
		},
		[messageApi, refreshAll],
	);

	const deleteSubscriptionItem = useCallback(
		async (item: Subscription) => {
			try {
				await api.deleteSubscription(item.id);
				messageApi.success(`已删除 ${item.name}`);
				await refreshAll();
			} catch (error) {
				messageApi.error(error instanceof Error ? error.message : "删除失败");
			}
		},
		[messageApi, refreshAll],
	);

	return (
		<Layout style={{ minHeight: "100vh", background: "#f8fafc" }}>
			{contextHolder}

			<Content style={{ padding: 24 }}>
				<Spin spinning={loading}>
					<Flex vertical gap={24}>
						<Row gutter={[16, 16]}>
							<Col xs={24} xl={12}>
								<Flex vertical gap={16} style={{ height: "100%" }}>
									<Card styles={{ body: { padding: 20 } }}>
										<Flex justify="space-between" align="flex-start" gap={16} wrap="wrap">
											<div>
												<Title level={3} style={{ margin: 0, color: "#0f172a" }}>
													Mihomo WebUI Proxy
												</Title>
												<Text type="secondary">订阅管理、配置生成、节点切换一体化面板</Text>
											</div>
											<Space wrap>
												<Button icon={<ReloadOutlined />} onClick={() => void refreshAll()}>
													刷新视图
												</Button>
												<Button
													type="primary"
													icon={<SyncOutlined spin={syncing} />}
													loading={syncing}
													onClick={async () => {
														setSyncing(true);
														try {
															const result = await api.syncConfig();
															if (result.reloaded) {
																messageApi.success("配置已重新生成并热加载到 mihomo");
															} else {
																messageApi.warning("配置已生成，但 mihomo controller 当前不可用，尚未热加载");
															}
															await refreshAll();
														} catch (error) {
															messageApi.error(error instanceof Error ? error.message : "同步失败");
														} finally {
															setSyncing(false);
														}
													}}
												>
													同步配置
												</Button>
											</Space>
										</Flex>
									</Card>

									<Row gutter={[16, 16]}>
                    <Col xs={24} md={8} style={{ display: "flex" }}>
                      <Card style={{ width: "100%", height: "100%" }}>
                        <Statistic
                          title="Mihomo 控制器"
                          value={status?.mihomoConnected ? "已连接" : "未连接"}
                          prefix={status?.mihomoConnected ? <CheckCircleOutlined /> : <CloseCircleOutlined />}
                          valueStyle={{ color: status?.mihomoConnected ? "#16a34a" : "#dc2626" }}
                        />
                        <Paragraph type="secondary" style={{ marginBottom: 0 }}>
                          {status?.mihomoVersion ? `版本 ${status.mihomoVersion}` : "等待 controller 就绪"}
                        </Paragraph>
                      </Card>
                    </Col>
                    <Col xs={24} md={8} style={{ display: "flex" }}>
                      <Card style={{ width: "100%", height: "100%" }}>
                        <Statistic title="订阅数量" value={status?.subscriptionCount ?? 0} />
                        <Paragraph type="secondary" style={{ marginBottom: 0 }}>
                          最近配置生成：{formatDate(status?.lastConfigSyncAt)}
                        </Paragraph>
                      </Card>
                    </Col>
                    <Col xs={24} md={8} style={{ display: "flex" }}>
                      <Card style={{ width: "100%", height: "100%" }}>
                        <Statistic title="配置路径" value={status?.configPath ?? "-"} valueStyle={{ fontSize: 16 }} />
                        <Paragraph type="secondary" style={{ marginBottom: 0 }}>
                          最近错误：{status?.lastConfigError || "无"}
                        </Paragraph>
                      </Card>
                    </Col>
                  </Row>

                  <Card
                    title="订阅列表"
                    extra={
                      <Button
                        type="primary"
                        icon={<PlusOutlined />}
                        onClick={() => {
                          if (subscriptionPanelOpen && !editing) {
                            closeSubscriptionPanel();
                            return;
                          }
                          openCreateSubscription();
                        }}
                      >
                        {subscriptionPanelOpen && !editing ? "收起" : "添加订阅"}
                      </Button>
                    }
                    style={{ flex: 1 }}
                  >
                    {subscriptionPanelOpen ? (
                      <Card size="small" style={{ marginBottom: 16, background: "#f8fafc", borderColor: "#dbeafe" }}>
                        <Flex justify="space-between" align="center" wrap="wrap" gap={12} style={{ marginBottom: 16 }}>
                          <div>
                            <Text strong style={{ fontSize: 16 }}>{editing ? `编辑订阅：${editing.name}` : "添加订阅"}</Text>
                            <br />
                            <Text type="secondary">保存后会自动刷新列表，并在可用时同步到 mihomo。</Text>
                          </div>
                          <Space>
                            {editing ? (
                              <Button
                                onClick={() => {
                                  setEditing(null);
                                  form.resetFields();
                                }}
                              >
                                切换为新增
                              </Button>
                            ) : null}
                            <Button onClick={closeSubscriptionPanel}>关闭</Button>
                          </Space>
                        </Flex>

                        <Form
                          layout="vertical"
                          form={form}
                          initialValues={{ enabled: true }}
                          onFinish={(values) => void onFinish(values)}
                        >
                          <Form.Item label="名称" name="name" rules={[{ required: true, message: "请输入订阅名称" }]}> 
                            <Input placeholder="例如：机场 A" />
                          </Form.Item>
                          <Form.Item label="订阅链接" name="url" rules={[{ required: true, message: "请输入订阅链接" }]}> 
                            <Input.TextArea placeholder="https://example.com/subscription.yaml" autoSize={{ minRows: 3, maxRows: 5 }} />
                          </Form.Item>
                          <Form.Item label="启用" name="enabled" valuePropName="checked">
                            <Switch checkedChildren="启用" unCheckedChildren="停用" />
                          </Form.Item>
                          <Space>
                            <Button type="primary" htmlType="submit" icon={<PlusOutlined />} loading={submitting}>
                              {editing ? "保存修改" : "添加并拉取"}
                            </Button>
                            <Button onClick={() => form.resetFields()}>重置</Button>
                          </Space>
                        </Form>
                      </Card>
                    ) : null}

									{subscriptions.length === 0 ? (
										<Empty description="暂无订阅，点击右上角添加订阅开始使用" />
									) : (
										<List
											dataSource={subscriptions}
											renderItem={(item) => (
												<List.Item style={{ padding: 0, border: 0, marginBottom: 12 }}>
													<Card size="small" style={{ width: "100%", borderColor: item.enabled ? "#cbd5e1" : "#e5e7eb" }}>
														<Flex vertical gap={14}>
															<Flex justify="space-between" align="flex-start" gap={16} wrap="wrap">
																<div style={{ minWidth: 0, flex: 1 }}>
																	<Flex align="center" gap={10} wrap="wrap">
																		<Text strong style={{ fontSize: 16 }}>{item.name}</Text>
																		{item.enabled ? <Tag color="green">启用</Tag> : <Tag>停用</Tag>}
																	</Flex>
																	<Tooltip title={item.url}>
																		<Text type="secondary" ellipsis style={{ maxWidth: "100%", display: "inline-block", marginTop: 4 }}>
																			{item.url}
																		</Text>
																	</Tooltip>
																</div>
																<Space size={6} wrap>
																	<Button size="small" type="text" onClick={() => void openSubscriptionViewer(item)}>
																		查看内容
																	</Button>
																	<Button size="small" type="text" onClick={() => editSubscription(item)}>
																		编辑
																	</Button>
																	<Button size="small" type="text" icon={<ReloadOutlined />} onClick={() => void refreshSubscriptionItem(item)}>
																		刷新
																	</Button>
																	<Popconfirm title="确认删除该订阅？" onConfirm={() => void deleteSubscriptionItem(item)}>
																		<Button size="small" type="text" danger>
																			删除
																		</Button>
																	</Popconfirm>
																</Space>
															</Flex>
															<Flex gap={10} wrap="wrap">
																<Tag color="default">最近刷新：{formatDate(item.lastRefreshedAt)}</Tag>
																<Tag color="default">文件：{item.filePath || "-"}</Tag>
																<Tag color={item.lastError ? "error" : "success"}>
																	{item.lastError ? `最近错误：${item.lastError}` : "最近错误：无"}
																</Tag>
															</Flex>
														</Flex>
													</Card>
												</List.Item>
											)}
										/>
									)}
								</Card>
                </Flex>
              </Col>

              <Col xs={24} xl={12}>
                <Card
                  title={
                    <Space size={8}>
                      <span>代理组与节点</span>
                      <Tooltip
                        title={
                          hiddenGlobalGroup
                            ? "GLOBAL 已隐藏。当前页面按 rule 模式展示实际有用的切换入口，真正决定流量的是 Proxy 组。"
                            : "当前页面展示实际有用的代理组切换入口。"
                        }
                      >
                        <InfoCircleOutlined style={{ color: "#64748b" }} />
                      </Tooltip>
                    </Space>
                  }
                  extra={<Button size="small" onClick={() => void refreshAll()}>刷新代理组</Button>}
                  styles={{ body: { padding: 18 } }}
                  style={{ height: "100%" }}
                >
                  {proxyGroupError ? <Alert type="warning" showIcon message="无法读取 mihomo 代理组" description={proxyGroupError} style={{ marginBottom: 16 }} /> : null}
                  {visibleProxyGroups.length === 0 ? (
                    <Empty description={proxyGroupError ? "mihomo controller 不在线或未成功热加载配置" : "暂无代理组，请先导入有效订阅并等待 mihomo 加载成功"} />
                  ) : (
                    <Flex vertical gap={16}>
                      <Segmented
                        block
                        value={activeProxyGroup?.name}
                        onChange={(value) => setActiveProxyGroupName(String(value))}
                        options={visibleProxyGroups.map((group) => ({
                          label: group.name,
                          value: group.name,
                        }))}
                      />

						{activeProxyGroup ? (() => {
							const options = uniqueItems(activeProxyGroup.all);
							return (
                          <Card
                            key={activeProxyGroup.name}
                            size="small"
                            style={{
                              borderColor: activeProxyGroup.name === "Proxy" ? "#0f766e" : "#d4d4d8",
                              background: activeProxyGroup.name === "Proxy"
                                ? "linear-gradient(135deg, #f0fdfa 0%, #ffffff 100%)"
                                : "linear-gradient(135deg, #f8fafc 0%, #ffffff 100%)",
                            }}
                            styles={{ body: { padding: 16 } }}
                          >
                            <Space direction="vertical" size={14} style={{ width: "100%" }}>
                              <Flex justify="space-between" align="flex-start" gap={12} wrap="wrap">
                                <div>
                                  <Text strong style={{ fontSize: 16 }}>{activeProxyGroup.name}</Text>
                                  <br />
                                  <Text type="secondary">当前节点：{activeProxyGroup.current || "未选择"}</Text>
                                </div>
                                <Space size={8} wrap>
                                  <Tag color={activeProxyGroup.name === "Proxy" ? "cyan" : "gold"}>{activeProxyGroup.type}</Tag>
                                  <Tag color="default">{options.length} 个节点</Tag>
                                </Space>
                              </Flex>

                              <div
                                style={{
                                  display: "grid",
                                  gridTemplateColumns: "repeat(auto-fit, minmax(150px, 1fr))",
                                  gap: 10,
                                }}
                              >
													{options.map((item) => {
														const selected = activeProxyGroup.current === item;
														const busy = selectingGroup === activeProxyGroup.name;
														const sourceLabel = formatNodeSourceLabel(item, activeProxyGroup.nodeSources?.[item]);
														return (
															<Button
                                      key={`${activeProxyGroup.name}-${item}`}
                                      type={selected ? "primary" : "default"}
                                      ghost={!selected}
                                      loading={busy && selected}
                                      disabled={busy}
                                      onClick={() => void selectProxyNode(activeProxyGroup.name, item)}
                                      style={{
                                        height: "auto",
                                        minHeight: 68,
                                        padding: "12px 14px",
                                        textAlign: "left",
                                        justifyContent: "flex-start",
                                        whiteSpace: "normal",
                                        borderColor: selected ? undefined : "#cbd5e1",
                                        background: selected ? undefined : "rgba(255,255,255,0.94)",
                                      }}
                                    >
                                      <Space direction="vertical" size={4} style={{ width: "100%" }}>
                                        <Text
                                          strong={selected}
                                          style={{
                                            color: selected ? "#ffffff" : "#0f172a",
                                            lineHeight: 1.35,
                                          }}
                                        >
                                          {item}
                                        </Text>
																<Text style={{ color: selected ? "rgba(255,255,255,0.78)" : "#64748b", fontSize: 12 }}>
																	{sourceLabel}
																</Text>
															</Space>
															</Button>
                                  );
                                })}
                              </div>
                            </Space>
                          </Card>
                        );
                      })() : null}
                    </Flex>
                  )}
                </Card>
              </Col>
            </Row>

          </Flex>
        </Spin>

        <Drawer
          title={viewerData ? `订阅内容：${viewerData.name}` : "订阅内容"}
          open={viewerOpen}
          width={720}
          onClose={() => setViewerOpen(false)}
        >
          <Spin spinning={viewerLoading}>
            <Space direction="vertical" size={12} style={{ width: "100%" }}>
              <Text type="secondary">{viewerData?.filePath}</Text>
              <Input.TextArea value={viewerData?.content ?? ""} readOnly autoSize={{ minRows: 24, maxRows: 30 }} />
            </Space>
          </Spin>
        </Drawer>
      </Content>
    </Layout>
  );
}

export default function App() {
  return (
    <ConfigProvider
      theme={{
        token: {
          colorPrimary: "#0f766e",
          borderRadius: 12,
        },
      }}
    >
      <AntApp>
        <AppInner />
      </AntApp>
    </ConfigProvider>
  );
}

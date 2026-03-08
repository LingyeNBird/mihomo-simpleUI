import {
  CheckCircleOutlined,
  CloseCircleOutlined,
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
  Select,
  Space,
  Spin,
  Statistic,
  Switch,
  Table,
  Tag,
  Typography,
} from "antd";
import type { ColumnsType } from "antd/es/table";
import { useCallback, useEffect, useMemo, useState } from "react";
import { api } from "./api";
import type { AppStatus, ProxyGroup, Subscription, SubscriptionContent } from "./types";

const { Header, Content } = Layout;
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
  const [syncing, setSyncing] = useState(false);

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
      await refreshAll();
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : "保存失败");
    } finally {
      setSubmitting(false);
    }
  };

  const columns: ColumnsType<Subscription> = useMemo(
    () => [
      { title: "名称", dataIndex: "name", key: "name" },
      {
        title: "状态",
        key: "enabled",
        render: (_, item) =>
          item.enabled ? <Tag color="green">启用</Tag> : <Tag color="default">停用</Tag>,
      },
      {
        title: "最近刷新",
        key: "lastRefreshedAt",
        render: (_, item) => formatDate(item.lastRefreshedAt),
      },
      {
        title: "最近错误",
        key: "lastError",
        render: (_, item) =>
          item.lastError ? <Text type="danger">{item.lastError}</Text> : <Text type="secondary">-</Text>,
      },
      {
        title: "操作",
        key: "actions",
        render: (_, item) => (
          <Space wrap>
            <Button
              size="small"
              onClick={async () => {
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
              }}
            >
              查看内容
            </Button>
            <Button
              size="small"
              onClick={() => {
                setEditing(item);
                form.setFieldsValue({ name: item.name, url: item.url, enabled: item.enabled });
              }}
            >
              编辑
            </Button>
            <Button
              size="small"
              icon={<ReloadOutlined />}
              onClick={async () => {
                try {
                  await api.refreshSubscription(item.id);
                  messageApi.success(`已刷新 ${item.name}`);
                  await refreshAll();
                } catch (error) {
                  messageApi.error(error instanceof Error ? error.message : "刷新失败");
                }
              }}
            >
              刷新
            </Button>
            <Popconfirm
              title="确认删除该订阅？"
              onConfirm={async () => {
                try {
                  await api.deleteSubscription(item.id);
                  messageApi.success(`已删除 ${item.name}`);
                  await refreshAll();
                } catch (error) {
                  messageApi.error(error instanceof Error ? error.message : "删除失败");
                }
              }}
            >
              <Button size="small" danger>
                删除
              </Button>
            </Popconfirm>
          </Space>
        ),
      },
    ],
    [form, messageApi, refreshAll],
  );

  return (
    <Layout style={{ minHeight: "100vh" }}>
      {contextHolder}
      <Header
        style={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          background: "rgba(15, 23, 42, 0.9)",
          backdropFilter: "blur(12px)",
        }}
      >
        <div>
          <Title level={3} style={{ color: "white", margin: 0 }}>
            Mihomo WebUI Proxy
          </Title>
          <Text style={{ color: "rgba(255,255,255,0.72)" }}>订阅管理、配置生成、节点切换一体化面板</Text>
        </div>
        <Space>
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
      </Header>

      <Content style={{ padding: 24 }}>
        <Spin spinning={loading}>
          <Flex vertical gap={24}>
            <Row gutter={[16, 16]}>
              <Col xs={24} md={8}>
                <Card>
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
              <Col xs={24} md={8}>
                <Card>
                  <Statistic title="订阅数量" value={status?.subscriptionCount ?? 0} />
                  <Paragraph type="secondary" style={{ marginBottom: 0 }}>
                    最近配置生成：{formatDate(status?.lastConfigSyncAt)}
                  </Paragraph>
                </Card>
              </Col>
              <Col xs={24} md={8}>
                <Card>
                  <Statistic title="配置路径" value={status?.configPath ?? "-"} valueStyle={{ fontSize: 16 }} />
                  <Paragraph type="secondary" style={{ marginBottom: 0 }}>
                    最近错误：{status?.lastConfigError || "无"}
                  </Paragraph>
                </Card>
              </Col>
            </Row>

            <Row gutter={[16, 16]}>
              <Col xs={24} xl={14}>
                <Card title={editing ? `编辑订阅：${editing.name}` : "添加订阅"} extra={editing ? <Button onClick={() => { setEditing(null); form.resetFields(); }}>取消编辑</Button> : null}>
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
              </Col>

              <Col xs={24} xl={10}>
                <Card title="代理组与节点" extra={<Button size="small" onClick={() => void refreshAll()}>刷新代理组</Button>}>
                  {proxyGroupError ? <Alert type="warning" showIcon message="无法读取 mihomo 代理组" description={proxyGroupError} style={{ marginBottom: 16 }} /> : null}
                  {proxyGroups.length === 0 ? (
                    <Empty description={proxyGroupError ? "mihomo controller 不在线或未成功热加载配置" : "暂无代理组，请先导入有效订阅并等待 mihomo 加载成功"} />
                  ) : (
                    <List
                      itemLayout="vertical"
                      dataSource={proxyGroups}
                      renderItem={(group) => (
                        <List.Item key={group.name}>
                          <Space direction="vertical" size={12} style={{ width: "100%" }}>
                            <Flex justify="space-between" align="center">
                              <div>
                                <Text strong>{group.name}</Text>
                                <br />
                                <Text type="secondary">当前节点：{group.current || "未选择"}</Text>
                              </div>
                              <Tag color="blue">{group.type}</Tag>
                            </Flex>
                            <Select
                              value={group.current || undefined}
                              style={{ width: "100%" }}
                              options={group.all.map((item) => ({ label: item, value: item }))}
                              placeholder="选择节点"
                              onChange={async (value) => {
                                try {
                                  await api.selectNode(group.name, value);
                                  messageApi.success(`已将 ${group.name} 切换到 ${value}`);
                                  await refreshAll();
                                } catch (error) {
                                  messageApi.error(error instanceof Error ? error.message : "切换失败");
                                }
                              }}
                            />
                          </Space>
                        </List.Item>
                      )}
                    />
                  )}
                </Card>
              </Col>
            </Row>

            <Card title="订阅列表">
              <Table rowKey="id" columns={columns} dataSource={subscriptions} pagination={false} scroll={{ x: 960 }} />
            </Card>
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
          colorPrimary: "#4f46e5",
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

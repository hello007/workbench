const { execSync } = require('child_process');
const fs = require('fs');
const path = require('path');
const https = require('https');
const http = require('http');

// ============================================
// 配置和常量
// ============================================

const SCRIPT_DIR = __dirname;
const ERROR_LOG = path.join(SCRIPT_DIR, 'notify_error.log');
const CONFIG_FILE = path.join(SCRIPT_DIR, 'notify_config.json');
const STYLES_FILE = path.join(SCRIPT_DIR, 'ui_styles.json');

const EVENT_PERMISSION_REQUEST = 'PermissionRequest';
const EVENT_STOP = 'Stop';
const EVENT_PRE_TOOL_USE = 'PreToolUse';
const EVENT_POST_TOOL_USE = 'PostToolUse';
const TOOL_ASK_USER_QUESTION = 'AskUserQuestion';

const SOUND_EXCLAMATION = 'Exclamation';
const SOUND_ASTERISK = 'Asterisk';
const SOUND_QUESTION = 'Question';

function loadConfig() {
  const defaults = {
    notify_url: '',
    sound_enabled: true,
    log_enabled: true
  };

  try {
    if (fs.existsSync(CONFIG_FILE)) {
      const userConfig = JSON.parse(fs.readFileSync(CONFIG_FILE, 'utf8'));
      return { ...defaults, ...userConfig };
    }
  } catch (err) {
    // 使用默认配置
  }

  return defaults;
}

const CONFIG = loadConfig();

// ============================================
// UI 样式配置
// ============================================

function loadStyles() {
  const defaults = {
    colors: {
      primary: '#4361ee',
      success: '#10b981',
      danger: '#ef4444',
      background: '#f8f9fa',
      containerBackground: '#f8f9fa',
      cardBackground: '#ffffff',
      metaBackground: '#e9ecef',
      border: '#dee2e6',
      labelText: '#212529',
      valueText: '#495057',
      descText: '#6c757d',
      white: '#ffffff',
      buttonHover: '#f8f9fa',
      secondaryHover: '#ced4da',
      shadowColor: '#000000'
    },
    fontSizes: {
      meta: 13,
      title: 13,
      content: 12,
      question: 12,
      optionLabel: 11,
      optionDesc: 10,
      message: 12,
      button: 12,
      customLabel: 9,
      customInput: 10
    },
    fontFamily: {
      text: 'Microsoft YaHei, Segoe UI',
      mono: 'Consolas'
    },
    cornerRadius: {
      card: 8,
      meta: 6,
      button: 6,
      option: 6
    },
    shadow: {
      blurRadius: 20,
      shadowDepth: 4,
      opacity: 0.12,
      direction: 315
    },
    spacing: {
      metaPadding: '20,16',
      metaLineSpacing: 2,
      contentPadding: '20',
      titleMargin: '20,18,20,12',
      questionPadding: '20,12,20,12',
      buttonMargin: '0,0,16,0',
      optionMargin: '0,0,0,8',
      optionPadding: '12',
      sectionMargin: '0,0,0,12',
      customInputMargin: '20,12,20,8',
      customLabelMargin: '0,0,0,8',
      separatorMargin: '0,0,0,12'
    },
    button: {
      minHeight: 36
    },
    window: {
      minWidth: 600,
      maxWidth: 800,
      minHeight: 200,
      maxHeight: 700
    }
  };

  try {
    if (fs.existsSync(STYLES_FILE)) {
      const userStyles = JSON.parse(fs.readFileSync(STYLES_FILE, 'utf8'));
      return {
        colors: { ...defaults.colors, ...userStyles.colors },
        fontSizes: { ...defaults.fontSizes, ...userStyles.fontSizes },
        fontFamily: { ...defaults.fontFamily, ...userStyles.fontFamily },
        cornerRadius: { ...defaults.cornerRadius, ...userStyles.cornerRadius },
        shadow: { ...defaults.shadow, ...userStyles.shadow },
        spacing: { ...defaults.spacing, ...userStyles.spacing },
        button: { ...defaults.button, ...userStyles.button },
        window: { ...defaults.window, ...userStyles.window }
      };
    }
  } catch (err) {
    logError(`[Styles] Load error: ${err.message}\n`);
  }

  return defaults;
}

const UI_STYLES = loadStyles();

// 预览模式：node notify.js --preview 时注入模拟数据依次渲染三弹窗，供目视验收
const PREVIEW_MODE = process.argv.includes('--preview');

// ============================================
// 工具函数
// ============================================

function logError(message) {
  if (CONFIG.log_enabled) {
    fs.appendFileSync(ERROR_LOG, message);
  }
}

function extractField(data, fieldName, defaultValue = '') {
  const standardMappings = {
    session_id: ['session_id', 'sessionId'],
    cwd: ['cwd', 'working_directory'],
    transcript_path: ['transcript_path', 'transcriptPath'],
    tool_name: ['tool_name', 'toolName'],
    tool_input: ['tool_input', 'toolInput']
  };

  const fallbacks = standardMappings[fieldName] || [fieldName];

  for (const key of fallbacks) {
    const value = data[key];
    if (value !== undefined && value !== null && value !== '') {
      return value;
    }
  }

  return defaultValue;
}

function encodeUtf8ToBase64(str) {
  return Buffer.from(str, 'utf8').toString('base64');
}

// 生成样式辅助函数
function getStyleHelpers() {
  const { colors, fontSizes, fontFamily, cornerRadius, shadow, spacing, button } = UI_STYLES;

  return {
    // 窗口基础样式
    window: `
$window.MinWidth = ${UI_STYLES.window.minWidth}
$window.MaxWidth = ${UI_STYLES.window.maxWidth}
$window.MinHeight = ${UI_STYLES.window.minHeight}
$window.MaxHeight = ${UI_STYLES.window.maxHeight}
$window.WindowStartupLocation = 'CenterScreen'
$window.Topmost = $$true
$window.Background = '${colors.background}'`,

    // 按钮样式（主按钮）
    primaryButton: (varName) => `
${varName}.FontSize = ${fontSizes.button}
${varName}.Padding = '12,8'
${varName}.Margin = '${spacing.buttonMargin}'
${varName}.Background = '${colors.primary}'
${varName}.Foreground = '${colors.white}'
${varName}.BorderThickness = '0'
${varName}.Cursor = 'Hand'
${varName}.add_MouseEnter({
  ${varName}.Opacity = 0.9
})
${varName}.add_MouseLeave({
  ${varName}.Opacity = 1.0
})`,

    // 按钮样式（次要按钮）
    secondaryButton: (varName) => `
${varName}.FontSize = ${fontSizes.button}
${varName}.Padding = '12,8'
${varName}.Margin = '${spacing.buttonMargin}'
${varName}.Background = '${colors.white}'
${varName}.Foreground = '${colors.labelText}'
${varName}.BorderBrush = '${colors.border}'
${varName}.BorderThickness = '1'
${varName}.Cursor = 'Hand'
${varName}.add_MouseEnter({
  ${varName}.Background = '${colors.buttonHover}'
})
${varName}.add_MouseLeave({
  ${varName}.Background = '${colors.white}'
})`,

    // 内容区域样式
    contentPadding: spacing.contentPadding,
    contentFontSize: fontSizes.content,
    messageFontSize: fontSizes.message,

    // 颜色快速访问
    colors,
    fontSizes,
    fontFamily,
    cornerRadius,
    shadow,
    spacing,
    // 卡片阴影效果（DropShadowEffect）
    cardEffect: (varName) => `
$${varName}Effect = New-Object System.Windows.Media.Effects.DropShadowEffect
$${varName}Effect.Color = [System.Windows.Media.ColorConverter]::ConvertFromString('${colors.shadowColor}')
$${varName}Effect.BlurRadius = ${shadow.blurRadius}
$${varName}Effect.ShadowDepth = ${shadow.shadowDepth}
$${varName}Effect.Opacity = ${shadow.opacity}
$${varName}Effect.Direction = ${shadow.direction}
$${varName}.Effect = $${varName}Effect`,
    // 圆角按钮样式（注入 ControlTemplate 实现 CornerRadius，variant: primary/secondary/success/danger）
    buttonStyle: (varName, variant) => {
      const isSecondary = variant === 'secondary';
      const bg = isSecondary ? colors.metaBackground
        : (variant === 'primary' ? colors.primary
          : variant === 'success' ? colors.success : colors.danger);
      const fg = isSecondary ? colors.labelText : colors.white;
      const borderBrush = colors.border;
      const borderThickness = 0;
      const hoverCode = isSecondary
        ? `$${varName}.add_MouseEnter({ $this.Background = '${colors.secondaryHover}' })
$${varName}.add_MouseLeave({ $this.Background = '${colors.metaBackground}' })`
        : `$${varName}.add_MouseEnter({ $this.Opacity = 0.9 })
$${varName}.add_MouseLeave({ $this.Opacity = 1.0 })`;
      return `
$${varName}.Background = '${bg}'
$${varName}.Foreground = '${fg}'
$${varName}.BorderBrush = '${borderBrush}'
$${varName}.BorderThickness = '${borderThickness}'
$${varName}.Padding = '12,8'
$${varName}.MinHeight = ${button.minHeight}
$${varName}.Margin = '${spacing.buttonMargin}'
$${varName}.FontSize = ${fontSizes.button}
$${varName}.FontWeight = 'Bold'
$${varName}.Cursor = 'Hand'
$${varName}.FontFamily = '${fontFamily.text}'
$${varName}Tpl = [System.Windows.Markup.XamlReader]::Parse('<ControlTemplate TargetType="Button" xmlns="http://schemas.microsoft.com/winfx/2006/xaml/presentation"><Border Background="{TemplateBinding Background}" BorderBrush="{TemplateBinding BorderBrush}" BorderThickness="{TemplateBinding BorderThickness}" CornerRadius="${cornerRadius.button}" Padding="{TemplateBinding Padding}"><ContentPresenter HorizontalAlignment="Center" VerticalAlignment="Center"/></Border></ControlTemplate>')
$${varName}.Template = $${varName}Tpl
${hoverCode}`;
    },
    // 字体设置（text 正文 / mono 等宽）
    textFont: (varName, type) => `$${varName}.FontFamily = '${type === 'mono' ? fontFamily.mono : fontFamily.text}'`,
    // 选项按钮样式（圆角 ControlTemplate + 左对齐 + 白底边框 + hover，用于 AskUserQuestion 单选项）
    optionButtonStyle: (varName) => `
$${varName}.Background = '${colors.white}'
$${varName}.Foreground = '${colors.labelText}'
$${varName}.BorderBrush = '${colors.border}'
$${varName}.BorderThickness = '1'
$${varName}.Padding = '8'
$${varName}.Margin = '0,0,0,4'
$${varName}.Cursor = 'Hand'
$${varName}.HorizontalContentAlignment = 'Left'
$${varName}.FontFamily = '${fontFamily.text}'
$${varName}Tpl = [System.Windows.Markup.XamlReader]::Parse('<ControlTemplate TargetType="Button" xmlns="http://schemas.microsoft.com/winfx/2006/xaml/presentation"><Border Background="{TemplateBinding Background}" BorderBrush="{TemplateBinding BorderBrush}" BorderThickness="{TemplateBinding BorderThickness}" CornerRadius="${cornerRadius.option}" Padding="{TemplateBinding Padding}"><ContentPresenter HorizontalAlignment="Left" VerticalAlignment="Center"/></Border></ControlTemplate>')
$${varName}.Template = $${varName}Tpl
$${varName}.add_MouseEnter({ $this.Background = '${colors.buttonHover}' })
$${varName}.add_MouseLeave({ $this.Background = '${colors.white}' })`
  };
}

// 生成统一的元信息栏 PowerShell 代码
function generateMetaInfoBar(sessionId, cwd, additionalFields = []) {
  const projectName = cwd ? path.basename(cwd) : 'Unknown';
  const sessionShort = sessionId ? sessionId.substring(0, 8) : 'Unknown';

  const projectLabel_b64 = encodeUtf8ToBase64('📦 项目');
  const sessionLabel_b64 = encodeUtf8ToBase64('🔖 会话');
  const pathLabel_b64 = encodeUtf8ToBase64('📁 路径');

  const { colors, fontSizes, fontFamily, cornerRadius, spacing } = UI_STYLES;

  const additionalRows = additionalFields.map(field => {
    const label_b64 = encodeUtf8ToBase64(field.label);
    const value = field.value.replace(/\\/g, '\\\\').replace(/'/g, "''");
    return `
$${field.id}Row = New-Object System.Windows.Controls.StackPanel
$${field.id}Row.Orientation = 'Horizontal'
$${field.id}Row.Margin = '0,0,0,${spacing.metaLineSpacing}'
$${field.id}Lbl = New-Object System.Windows.Controls.TextBlock
$${field.id}Lbl.Text = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('${label_b64}')) + ': '
$${field.id}Lbl.FontSize = ${fontSizes.meta}; $${field.id}Lbl.FontWeight = 'Bold'; $${field.id}Lbl.Foreground = '${colors.labelText}'; $${field.id}Lbl.FontFamily = '${fontFamily.text}'
$${field.id}Val = New-Object System.Windows.Controls.TextBlock
$${field.id}Val.Text = '${value}'
$${field.id}Val.FontSize = ${fontSizes.meta}; $${field.id}Val.Foreground = '${colors.valueText}'; $${field.id}Val.FontFamily = '${fontFamily.mono}'
${field.wrap ? `$${field.id}Val.TextWrapping = 'Wrap'` : ''}
$${field.id}Row.AddChild($${field.id}Lbl); $${field.id}Row.AddChild($${field.id}Val)
$metaPanel.AddChild($${field.id}Row)
`;
  }).join('\n');

  return `
# Meta info bar
$metaBar = New-Object System.Windows.Controls.Border
$metaBar.Background = '${colors.metaBackground}'
$metaBar.Padding = '${spacing.metaPadding}'
$metaBar.BorderBrush = '${colors.border}'
$metaBar.BorderThickness = '0,1,0,1'
$metaBar.CornerRadius = '${cornerRadius.meta}'
[System.Windows.Controls.Grid]::SetRow($metaBar, 1)

$metaPanel = New-Object System.Windows.Controls.StackPanel

$projectRow = New-Object System.Windows.Controls.StackPanel
$projectRow.Orientation = 'Horizontal'
$projectRow.Margin = '0,0,0,${spacing.metaLineSpacing}'
$projectLbl = New-Object System.Windows.Controls.TextBlock
$projectLbl.Text = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('${projectLabel_b64}')) + ': '
$projectLbl.FontSize = ${fontSizes.meta}; $projectLbl.FontWeight = 'Bold'; $projectLbl.Foreground = '${colors.labelText}'; $projectLbl.FontFamily = '${fontFamily.text}'
$projectVal = New-Object System.Windows.Controls.TextBlock
$projectVal.Text = '${projectName.replace(/'/g, "''")}'
$projectVal.FontSize = ${fontSizes.meta}; $projectVal.Foreground = '${colors.valueText}'; $projectVal.FontFamily = '${fontFamily.mono}'
$projectRow.AddChild($projectLbl); $projectRow.AddChild($projectVal)
$metaPanel.AddChild($projectRow)

$sessionRow = New-Object System.Windows.Controls.StackPanel
$sessionRow.Orientation = 'Horizontal'
$sessionRow.Margin = '0,0,0,${spacing.metaLineSpacing}'
$sessionLbl = New-Object System.Windows.Controls.TextBlock
$sessionLbl.Text = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('${sessionLabel_b64}')) + ': '
$sessionLbl.FontSize = ${fontSizes.meta}; $sessionLbl.FontWeight = 'Bold'; $sessionLbl.Foreground = '${colors.labelText}'; $sessionLbl.FontFamily = '${fontFamily.text}'
$sessionVal = New-Object System.Windows.Controls.TextBlock
$sessionVal.Text = '${sessionShort}'
$sessionVal.FontSize = ${fontSizes.meta}; $sessionVal.Foreground = '${colors.valueText}'; $sessionVal.FontFamily = '${fontFamily.mono}'
$sessionRow.AddChild($sessionLbl); $sessionRow.AddChild($sessionVal)
$metaPanel.AddChild($sessionRow)

$pathRow = New-Object System.Windows.Controls.StackPanel
$pathRow.Orientation = 'Horizontal'
$pathRow.Margin = '0,0,0,${spacing.metaLineSpacing}'
$pathLbl = New-Object System.Windows.Controls.TextBlock
$pathLbl.Text = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('${pathLabel_b64}')) + ': '
$pathLbl.FontSize = ${fontSizes.meta}; $pathLbl.FontWeight = 'Bold'; $pathLbl.Foreground = '${colors.labelText}'; $pathLbl.FontFamily = '${fontFamily.text}'
$pathVal = New-Object System.Windows.Controls.TextBlock
$pathVal.Text = '${cwd.replace(/\\/g, '\\\\').replace(/'/g, "''")}'
$pathVal.FontSize = ${fontSizes.meta}; $pathVal.Foreground = '${colors.valueText}'; $pathVal.FontFamily = '${fontFamily.mono}'
$pathVal.TextWrapping = 'Wrap'
$pathRow.AddChild($pathLbl); $pathRow.AddChild($pathVal)
$metaPanel.AddChild($pathRow)

${additionalRows}

$metaBar.Child = $metaPanel
$grid.AddChild($metaBar)
`;
}

function executePowerShellScript(scriptContent) {
  const tempFile = path.join(SCRIPT_DIR, `temp_ps_${Date.now()}.ps1`);

  try {
    // 写入 UTF-8 BOM 以确保 PowerShell 正确识别编码
    const utf8BOM = Buffer.from([0xEF, 0xBB, 0xBF]);
    const scriptBuffer = Buffer.from(scriptContent, 'utf8');
    const fileBuffer = Buffer.concat([utf8BOM, scriptBuffer]);
    fs.writeFileSync(tempFile, fileBuffer);

    // 执行 PowerShell 脚本
    const result = execSync(`powershell -NoProfile -ExecutionPolicy Bypass -File "${tempFile}"`, {
      encoding: 'utf8',
      timeout: 300000,
      windowsHide: false
    });
    return result.trim();
  } finally {
    try {
      if (fs.existsSync(tempFile)) {
        fs.unlinkSync(tempFile);
      }
    } catch (err) {
      // 忽略清理错误
    }
  }
}

function playSound(soundType) {
  if (!CONFIG.sound_enabled) return;

  try {
    execSync(`powershell -Command "[System.Media.SystemSounds]::${soundType}.Play()"`, {
      windowsHide: true
    });
  } catch (err) {
    // 忽略错误
  }
}

function sendHttpNotification(data) {
  const notifyUrl = (CONFIG.notify_url || '').trim();
  if (!notifyUrl) return;

  try {
    const sessionId = extractField(data, 'session_id', '');
    const cwd = extractField(data, 'cwd', '');
    const projectName = cwd ? path.basename(cwd) : '';
    const sessionShort = sessionId ? sessionId.substring(0, 8) : '';

    const msgParts = ['🤖 Claude 已完成任务'];
    if (projectName) msgParts.push(`📁 项目: ${projectName}`);
    if (sessionShort) msgParts.push(`🔑 会话: ${sessionShort}`);
    if (cwd) msgParts.push(`📂 目录: ${cwd}`);

    const msg = msgParts.join('\n');
    const urlObj = new URL(notifyUrl);
    const isHttps = urlObj.protocol === 'https:';
    const lib = isHttps ? https : http;

    const options = {
      hostname: urlObj.hostname,
      port: urlObj.port || (isHttps ? 443 : 80),
      path: urlObj.pathname + urlObj.search,
      method: 'POST',
      headers: {
        'Content-Type': 'text/plain',
        'Content-Length': Buffer.byteLength(msg)
      },
      timeout: 2000
    };

    const req = lib.request(options, (res) => {
      res.on('data', () => {});
    });

    req.on('error', () => {});
    req.on('timeout', () => req.destroy());
    req.write(msg);
    req.end();
  } catch (err) {
    // 忽略错误
  }
}

// ============================================
// PermissionRequest 处理
// ============================================

function handlePermissionRequest(data) {
  const toolName = extractField(data, 'tool_name', 'Unknown');

  // AskUserQuestion：不返回决策，让 Claude Code 走默认流显示原生面板。
  // 不可返回 decision.behavior:'allow'——对 AskUserQuestion 而言 allow-alone
  // 不充分（官方明文），会导致"放行了无答案的提问"而静默未作答。
  // 正常路径下 PreToolUse 已用 allow+updatedInput.answers 接管、不会走到这里；
  // 走到这里说明用户点了"知道了"或弹窗出错，此时回退原生面板。
  if (toolName === TOOL_ASK_USER_QUESTION) {
    return;
  }

  playSound(SOUND_EXCLAMATION);

  const S = getStyleHelpers();
  const toolInput = extractField(data, 'tool_input', {});
  const suggestions = data.permission_suggestions || [];
  const cwd = extractField(data, 'cwd', '');
  const sessionId = extractField(data, 'session_id', '');

  const details = Object.entries(toolInput)
    .filter(([k]) => k !== 'description')
    .map(([k, v]) => `${k}: ${typeof v === 'object' ? JSON.stringify(v) : v}`)
    .join('\n');

  const title_b64 = encodeUtf8ToBase64('Claude Code · 权限申请');
  const needAuth_b64 = encodeUtf8ToBase64('需要授权执行操作');
  const toolLabel_b64 = encodeUtf8ToBase64('工具');
  const detailsLabel_b64 = encodeUtf8ToBase64('参数详情');
  const allowBtn_b64 = encodeUtf8ToBase64('同意');
  const rememberBtn_b64 = encodeUtf8ToBase64('同意并记住');
  const denyBtn_b64 = encodeUtf8ToBase64('拒绝');

  const psScript = `
# 强制使用 UTF-8 编码
[Console]::InputEncoding = [System.Text.Encoding]::UTF8
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$PSDefaultParameterValues['*:Encoding'] = 'utf8'
$OutputEncoding = [System.Text.Encoding]::UTF8

Add-Type -AssemblyName PresentationFramework
Add-Type -AssemblyName PresentationCore

[System.Media.SystemSounds]::Exclamation.Play()

$w = New-Object System.Windows.Window
$w.Title = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('${title_b64}'))
$w.Width = 700
$w.Height = 450
$w.WindowStartupLocation = 'CenterScreen'
$w.ResizeMode = 'NoResize'
$w.Background = '${S.colors.containerBackground}'
$w.Topmost = $true

$w.Add_Loaded({ $w.Activate(); $w.Focus() })

$grid = New-Object System.Windows.Controls.Grid
$grid.Margin = '0'

$row0 = New-Object System.Windows.Controls.RowDefinition; $row0.Height = 'Auto'
$row1 = New-Object System.Windows.Controls.RowDefinition; $row1.Height = 'Auto'
$row2 = New-Object System.Windows.Controls.RowDefinition; $row2.Height = '*'
$row3 = New-Object System.Windows.Controls.RowDefinition; $row3.Height = 'Auto'
$grid.RowDefinitions.Add($row0); $grid.RowDefinitions.Add($row1)
$grid.RowDefinitions.Add($row2); $grid.RowDefinitions.Add($row3)

# Title
$titlePanel = New-Object System.Windows.Controls.StackPanel
$titlePanel.Margin = '20,18,20,12'
[System.Windows.Controls.Grid]::SetRow($titlePanel, 0)

$titleText = New-Object System.Windows.Controls.TextBlock
$titleText.Text = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('${needAuth_b64}'))
$titleText.FontSize = ${S.fontSizes.title}; $titleText.FontWeight = 'Bold'; $titleText.Foreground = '${S.colors.labelText}'; $titleText.FontFamily = '${S.fontFamily.text}'
$titlePanel.AddChild($titleText)
$grid.AddChild($titlePanel)

${generateMetaInfoBar(sessionId, cwd)}

# Content
$contentFrame = New-Object System.Windows.Controls.Border
$contentFrame.Margin = '16,12,16,12'
$contentFrame.Background = '${S.colors.cardBackground}'
$contentFrame.CornerRadius = '${S.cornerRadius.card}'
$contentFrame.Padding = '16'
${S.cardEffect('contentFrame')}
[System.Windows.Controls.Grid]::SetRow($contentFrame, 2)

$scrollViewer = New-Object System.Windows.Controls.ScrollViewer
$scrollViewer.VerticalScrollBarVisibility = 'Auto'
$scrollViewer.HorizontalScrollBarVisibility = 'Disabled'

$card = New-Object System.Windows.Controls.StackPanel
$card.Margin = '0'

$toolLbl = New-Object System.Windows.Controls.TextBlock
$toolLbl.Text = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('${toolLabel_b64}'))
$toolLbl.FontSize = ${S.fontSizes.content}; $toolLbl.FontWeight = 'Bold'; $toolLbl.Foreground = '${S.colors.labelText}'; $toolLbl.FontFamily = '${S.fontFamily.text}'
$card.AddChild($toolLbl)

$toolBlock = New-Object System.Windows.Controls.TextBlock
$toolBlock.Text = "${toolName}"
$toolBlock.FontSize = ${S.fontSizes.content}; $toolBlock.FontWeight = 'Bold'
$toolBlock.Foreground = '${S.colors.valueText}'; $toolBlock.Margin = '0,4,0,12'
$toolBlock.FontFamily = '${S.fontFamily.mono}'
$card.AddChild($toolBlock)

$detailsLbl = New-Object System.Windows.Controls.TextBlock
$detailsLbl.Text = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('${detailsLabel_b64}'))
$detailsLbl.FontSize = ${S.fontSizes.content}; $detailsLbl.FontWeight = 'Bold'; $detailsLbl.Foreground = '${S.colors.labelText}'; $detailsLbl.FontFamily = '${S.fontFamily.text}'
$card.AddChild($detailsLbl)

$detailsBlock = New-Object System.Windows.Controls.TextBlock
$detailsBlock.Text = @'
${details}
'@
$detailsBlock.FontSize = ${S.fontSizes.content}; $detailsBlock.Foreground = '${S.colors.valueText}'
$detailsBlock.TextWrapping = 'Wrap'; $detailsBlock.FontFamily = '${S.fontFamily.mono}'
$detailsBlock.Margin = '0,4,0,0'
$card.AddChild($detailsBlock)

$scrollViewer.Content = $card
$contentFrame.Child = $scrollViewer
$grid.AddChild($contentFrame)

# Buttons
$btnPanel = New-Object System.Windows.Controls.StackPanel
$btnPanel.Orientation = 'Horizontal'
$btnPanel.HorizontalAlignment = 'Right'
$btnPanel.Margin = '16,12,16,16'
[System.Windows.Controls.Grid]::SetRow($btnPanel, 3)

$denyBtn = New-Object System.Windows.Controls.Button
$denyBtn.Content = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('${denyBtn_b64}'))
${S.buttonStyle('denyBtn', 'danger')}
$denyBtn.Add_Click({ $w.Tag = 'deny'; $w.DialogResult = $true; $w.Close() })
$btnPanel.AddChild($denyBtn)

$allowBtn = New-Object System.Windows.Controls.Button
$allowBtn.Content = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('${allowBtn_b64}'))
${S.buttonStyle('allowBtn', 'primary')}
$allowBtn.Add_Click({ $w.Tag = 'allow'; $w.DialogResult = $true; $w.Close() })
$btnPanel.AddChild($allowBtn)

$rememberBtn = New-Object System.Windows.Controls.Button
$rememberBtn.Content = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('${rememberBtn_b64}'))
${S.buttonStyle('rememberBtn', 'success')}
$rememberBtn.Add_Click({ $w.Tag = 'remember'; $w.DialogResult = $true; $w.Close() })
$btnPanel.AddChild($rememberBtn)

$grid.AddChild($btnPanel)

$w.Content = $grid
$null = $w.ShowDialog()
Write-Output $w.Tag
`;

  try {
    const result = executePowerShellScript(psScript);

    const output = {
      hookSpecificOutput: {
        hookEventName: 'PermissionRequest',
        decision: {}
      }
    };

    if (result === 'allow') {
      output.hookSpecificOutput.decision.behavior = 'allow';
    } else if (result === 'remember') {
      output.hookSpecificOutput.decision.behavior = 'allow';
      const sugg = suggestions[0] || {
        type: 'addRules',
        rules: [{ toolName }],
        behavior: 'allow',
        destination: 'projectSettings'
      };
      sugg.destination = 'projectSettings';
      output.hookSpecificOutput.decision.updatedPermissions = [sugg];
    } else {
      output.hookSpecificOutput.decision.behavior = 'deny';
    }

    if (!PREVIEW_MODE) console.log(JSON.stringify(output));
  } catch (err) {
    logError(`[PermissionRequest] Error: ${err.message}\n`);
  }
}

// ============================================
// PreToolUse 处理 (AskUserQuestion)
// ============================================

function handlePreToolUse(data) {
  const toolName = extractField(data, 'tool_name');

  if (toolName !== TOOL_ASK_USER_QUESTION) {
    return;
  }

  const S = getStyleHelpers();
  const toolInput = extractField(data, 'tool_input', {});
  const questions = toolInput.questions || [];
  const cwd = extractField(data, 'cwd', '');
  const sessionId = extractField(data, 'session_id', '');

  if (questions.length === 0) {
    return;
  }

  const q = questions[0];
  const options = q.options || [];
  const multiSelect = q.multiSelect === true;

  const modeLabel_b64 = encodeUtf8ToBase64('类型');
  const modeValue_b64 = encodeUtf8ToBase64(multiSelect ? '多选' : '单选');
  const header_b64 = encodeUtf8ToBase64(q.header || '问题');
  const question_b64 = encodeUtf8ToBase64(q.question || '');

  const additionalFields = [
    { id: 'qtype', label: '📝 类型', value: multiSelect ? '多选' : '单选', wrap: false }
  ];

  const optionButtons = options
    .map((opt, idx) => {
      const label_b64 = encodeUtf8ToBase64(opt.label || `选项 ${idx + 1}`);
      const desc_b64 = encodeUtf8ToBase64(opt.description || '');
      const labelEscaped = (opt.label || '').replace(/'/g, "''");

      // 选项内容（label + desc），单选 Button / 多选 CheckBox 共用
      const contentCode = `
$btnContent${idx} = New-Object System.Windows.Controls.StackPanel
$btnLabel${idx} = New-Object System.Windows.Controls.TextBlock
$btnLabel${idx}.Text = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('${label_b64}'))
$btnLabel${idx}.FontWeight = 'Bold'; $btnLabel${idx}.FontSize = ${S.fontSizes.optionLabel}; $btnLabel${idx}.Foreground = '${S.colors.labelText}'; $btnLabel${idx}.FontFamily = '${S.fontFamily.text}'
$btnContent${idx}.AddChild($btnLabel${idx})
$btnDesc${idx} = New-Object System.Windows.Controls.TextBlock
$btnDesc${idx}.Text = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('${desc_b64}'))
$btnDesc${idx}.FontSize = ${S.fontSizes.optionDesc}; $btnDesc${idx}.Foreground = '${S.colors.descText}'; $btnDesc${idx}.Margin = '0,3,0,0'; $btnDesc${idx}.TextWrapping = 'Wrap'; $btnDesc${idx}.FontFamily = '${S.fontFamily.text}'
$btnContent${idx}.AddChild($btnDesc${idx})
`;

      if (multiSelect) {
        // 多选：CheckBox，勾选后由"提交多选答案"统一收集，不立即关窗
        return `
$btn${idx} = New-Object System.Windows.Controls.CheckBox
$btn${idx}.Tag = '${labelEscaped}'
$btn${idx}.Margin = '0,0,0,4'; $btn${idx}.Padding = '8'
$btn${idx}.Background = '${S.colors.white}'; $btn${idx}.BorderBrush = '${S.colors.border}'; $btn${idx}.BorderThickness = '1'
$btn${idx}.Cursor = 'Hand'; $btn${idx}.FontFamily = '${S.fontFamily.text}'; $btn${idx}.VerticalContentAlignment = 'Center'
${contentCode}
$btn${idx}.Content = $btnContent${idx}
$optionsPanel.AddChild($btn${idx})
`;
      }

      // 单选：Button，点击即提交并关窗
      return `
$btn${idx} = New-Object System.Windows.Controls.Button
$btn${idx}.Tag = '${labelEscaped}'
${S.optionButtonStyle('btn' + idx)}
${contentCode}
$btn${idx}.Content = $btnContent${idx}
$btn${idx}.Add_Click({ $w.Tag = $this.Tag; $w.DialogResult = $true; $w.Close() })
$optionsPanel.AddChild($btn${idx})
`;
    })
    .join('\n');

  const customInputLabel_b64 = encodeUtf8ToBase64(multiSelect ? '补充自定义答案（与勾选项一起提交）:' : '或输入自定义答案:');
  const submitCustom_b64 = encodeUtf8ToBase64('提交自定义答案');
  const submitMulti_b64 = encodeUtf8ToBase64('提交多选答案');
  const knowBtn_b64 = encodeUtf8ToBase64('知道了（回到 CLI）');

  const psScript = `
# 强制使用 UTF-8 编码
[Console]::InputEncoding = [System.Text.Encoding]::UTF8
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$PSDefaultParameterValues['*:Encoding'] = 'utf8'
$OutputEncoding = [System.Text.Encoding]::UTF8

Add-Type -AssemblyName PresentationFramework
Add-Type -AssemblyName PresentationCore

[System.Media.SystemSounds]::Question.Play()

$w = New-Object System.Windows.Window
$w.Title = 'Claude Code · 问题'
$w.Width = 700
$w.Height = 650
$w.WindowStartupLocation = 'CenterScreen'
$w.ResizeMode = 'NoResize'
$w.Background = '${S.colors.containerBackground}'
$w.Tag = ''
$w.Topmost = $true

$w.Add_Loaded({ $w.Activate(); $w.Focus() })

$grid = New-Object System.Windows.Controls.Grid
$grid.Margin = '0'

$row0 = New-Object System.Windows.Controls.RowDefinition; $row0.Height = 'Auto'
$row1 = New-Object System.Windows.Controls.RowDefinition; $row1.Height = 'Auto'
$row2 = New-Object System.Windows.Controls.RowDefinition; $row2.Height = 'Auto'
$row3 = New-Object System.Windows.Controls.RowDefinition; $row3.Height = '*'
$row4 = New-Object System.Windows.Controls.RowDefinition; $row4.Height = 'Auto'
$row5 = New-Object System.Windows.Controls.RowDefinition; $row5.Height = 'Auto'
$grid.RowDefinitions.Add($row0); $grid.RowDefinitions.Add($row1)
$grid.RowDefinitions.Add($row2); $grid.RowDefinitions.Add($row3)
$grid.RowDefinitions.Add($row4); $grid.RowDefinitions.Add($row5)

# Title
$titlePanel = New-Object System.Windows.Controls.StackPanel
$titlePanel.Margin = '20,18,20,12'
[System.Windows.Controls.Grid]::SetRow($titlePanel, 0)

$titleBlock = New-Object System.Windows.Controls.TextBlock
$titleBlock.Text = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('${header_b64}'))
$titleBlock.FontSize = ${S.fontSizes.title}
$titleBlock.FontWeight = 'Bold'
$titleBlock.Foreground = '${S.colors.labelText}'
$titleBlock.FontFamily = '${S.fontFamily.text}'
$titlePanel.AddChild($titleBlock)
$grid.AddChild($titlePanel)

${generateMetaInfoBar(sessionId, cwd, additionalFields)}

# Question
$questionPanel = New-Object System.Windows.Controls.Border
$questionPanel.Padding = '20,12,20,12'
[System.Windows.Controls.Grid]::SetRow($questionPanel, 2)

$questionBlock = New-Object System.Windows.Controls.TextBlock
$questionBlock.Text = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('${question_b64}'))
$questionBlock.FontSize = ${S.fontSizes.question}
$questionBlock.FontWeight = 'Bold'
$questionBlock.Foreground = '${S.colors.labelText}'
$questionBlock.FontFamily = '${S.fontFamily.text}'
$questionBlock.TextWrapping = 'Wrap'
$questionPanel.Child = $questionBlock
$grid.AddChild($questionPanel)

# Options
$scrollViewer = New-Object System.Windows.Controls.ScrollViewer
$scrollViewer.VerticalScrollBarVisibility = 'Auto'
$scrollViewer.HorizontalScrollBarVisibility = 'Disabled'
$scrollViewer.Margin = '20,0,20,0'
[System.Windows.Controls.Grid]::SetRow($scrollViewer, 3)

$optionsPanel = New-Object System.Windows.Controls.StackPanel
${optionButtons}

$scrollViewer.Content = $optionsPanel
$grid.AddChild($scrollViewer)

# Custom input section
$customInputPanel = New-Object System.Windows.Controls.StackPanel
$customInputPanel.Margin = '20,12,20,8'
[System.Windows.Controls.Grid]::SetRow($customInputPanel, 4)

$separator2 = New-Object System.Windows.Controls.Border
$separator2.Height = 1
$separator2.Background = '${S.colors.border}'
$separator2.Margin = '0,0,0,12'
$customInputPanel.AddChild($separator2)

$customLabel = New-Object System.Windows.Controls.TextBlock
$customLabel.Text = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('${customInputLabel_b64}'))
$customLabel.FontSize = ${S.fontSizes.customLabel}
$customLabel.FontWeight = 'Bold'
$customLabel.Foreground = '${S.colors.labelText}'
$customLabel.FontFamily = '${S.fontFamily.text}'
$customLabel.Margin = '0,0,0,8'
$customInputPanel.AddChild($customLabel)

$customInput = New-Object System.Windows.Controls.TextBox
$customInput.Padding = '8'
$customInput.FontSize = ${S.fontSizes.customInput}
$customInput.FontFamily = '${S.fontFamily.text}'
$customInput.Background = '${S.colors.white}'
$customInput.BorderBrush = '${S.colors.border}'
$customInputPanel.AddChild($customInput)

$grid.AddChild($customInputPanel)

# Bottom buttons
$btnPanel = New-Object System.Windows.Controls.StackPanel
$btnPanel.Orientation = 'Horizontal'
$btnPanel.HorizontalAlignment = 'Right'
$btnPanel.Margin = '20,12,20,16'
[System.Windows.Controls.Grid]::SetRow($btnPanel, 5)

$knowBtn = New-Object System.Windows.Controls.Button
$knowBtn.Content = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('${knowBtn_b64}'))
${S.buttonStyle('knowBtn', 'secondary')}
$knowBtn.Add_Click({ $w.Tag = ''; $w.DialogResult = $false; $w.Close() })
$btnPanel.AddChild($knowBtn)

${multiSelect ? `
$submitMulti = New-Object System.Windows.Controls.Button
$submitMulti.Content = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('${submitMulti_b64}'))
${S.buttonStyle('submitMulti', 'primary')}
$submitMulti.Add_Click({
  $selected = @()
  foreach ($child in $optionsPanel.Children) {
    if ($child -is [System.Windows.Controls.CheckBox] -and $child.IsChecked -eq $true) {
      $selected += $child.Tag
    }
  }
  $customText = $customInput.Text.Trim()
  if ($customText -ne '') { $selected += $customText }
  if ($selected.Count -gt 0) {
    $json = $selected | ConvertTo-Json -Compress
    if ($selected.Count -eq 1) { $json = '[' + $json + ']' }
    $w.Tag = $json
    $w.DialogResult = $true
    $w.Close()
  }
})
$btnPanel.AddChild($submitMulti)
` : ''}

${!multiSelect ? `
$submitCustom = New-Object System.Windows.Controls.Button
$submitCustom.Content = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('${submitCustom_b64}'))
${S.buttonStyle('submitCustom', 'success')}
$submitCustom.Add_Click({
  $text = $customInput.Text.Trim()
  if ($text -ne '') {
    $w.Tag = $text
    $w.DialogResult = $true
    $w.Close()
  }
})
$btnPanel.AddChild($submitCustom)
` : ''}

$grid.AddChild($btnPanel)

$w.Content = $grid
$null = $w.ShowDialog()
Write-Output $w.Tag
`;

  try {
    const selectedOption = executePowerShellScript(psScript);

    if (selectedOption) {
      const answers = {};
      if (multiSelect) {
        // 多选：PowerShell 返回 JSON 数组字符串（如 ["a","b"]），parse 为 label 数组
        let selectedArr;
        try {
          selectedArr = JSON.parse(selectedOption);
          if (!Array.isArray(selectedArr)) selectedArr = [String(selectedArr)];
        } catch (err) {
          selectedArr = [selectedOption];
        }
        answers[q.question] = selectedArr;
      } else {
        answers[q.question] = selectedOption;
      }

      const output = {
        hookSpecificOutput: {
          hookEventName: 'PreToolUse',
          permissionDecision: 'allow',
          updatedInput: {
            ...toolInput,
            answers: answers
          }
        }
      };

      if (!PREVIEW_MODE) console.log(JSON.stringify(output));
    } else {
      // "知道了（回到 CLI）"路径：不返回任何决策（exit 0 无 JSON），
      // 让 Claude Code 走默认权限流、显示原生 AskUserQuestion 面板，
      // 以便用户回 CLI 查看上下文后在原生面板作答。
      // 不可返回 permissionDecision:'defer'——该值仅 claude -p 非交互模式生效，
      // 交互式 CLI 下会被忽略（官方 Hooks reference 明文），导致面板自动取消。
    }
  } catch (err) {
    logError(`[PreToolUse] Error: ${err.message}\n`);
    // 弹窗异常时不返回决策，走默认流显示原生面板（原 defer 在交互式 CLI 下无效）。
  }
}

// ============================================
// Stop 事件处理
// ============================================

function handleStop(data) {
  playSound(SOUND_ASTERISK);
  sendHttpNotification(data);

  const S = getStyleHelpers();
  const cwd = extractField(data, 'cwd', '');
  const sessionId = extractField(data, 'session_id', '');
  const effortLevel = data.effort?.level || '';
  const permissionMode = data.permission_mode || '';
  const lastMessage = data.last_assistant_message || '';
  const backgroundTasks = (data.background_tasks || []).length;

  const additionalFields = [];
  if (effortLevel) {
    additionalFields.push({ id: 'effort', label: '🧠 思考级别', value: effortLevel, wrap: false });
  }
  if (permissionMode) {
    additionalFields.push({ id: 'permission', label: '🔐 权限模式', value: permissionMode, wrap: false });
  }
  if (backgroundTasks > 0) {
    additionalFields.push({ id: 'tasks', label: '⏳ 后台任务', value: `${backgroundTasks} 个`, wrap: false });
  }

  const title_b64 = encodeUtf8ToBase64('Claude Code · 任务完成');
  const heading_b64 = encodeUtf8ToBase64('Claude 已完成，等待输入');
  const msgLabel_b64 = encodeUtf8ToBase64('最后消息');
  const btnText_b64 = encodeUtf8ToBase64('知道了');

  const psScript = `
# 强制使用 UTF-8 编码
[Console]::InputEncoding = [System.Text.Encoding]::UTF8
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$PSDefaultParameterValues['*:Encoding'] = 'utf8'
$OutputEncoding = [System.Text.Encoding]::UTF8

Add-Type -AssemblyName PresentationFramework
Add-Type -AssemblyName PresentationCore

$w = New-Object System.Windows.Window
$w.Title = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('${title_b64}'))
$w.Width = 600
$w.Height = ${lastMessage ? 450 : 350}
$w.WindowStartupLocation = 'CenterScreen'
$w.ResizeMode = 'NoResize'
$w.Background = '${S.colors.containerBackground}'

$grid = New-Object System.Windows.Controls.Grid
$grid.Margin = '0'

$row0 = New-Object System.Windows.Controls.RowDefinition; $row0.Height = 'Auto'
$row1 = New-Object System.Windows.Controls.RowDefinition; $row1.Height = 'Auto'
$row2 = New-Object System.Windows.Controls.RowDefinition; $row2.Height = '*'
$row3 = New-Object System.Windows.Controls.RowDefinition; $row3.Height = 'Auto'
$grid.RowDefinitions.Add($row0); $grid.RowDefinitions.Add($row1)
$grid.RowDefinitions.Add($row2); $grid.RowDefinitions.Add($row3)

# Title
$titlePanel = New-Object System.Windows.Controls.StackPanel
$titlePanel.Margin = '20,18,20,12'
[System.Windows.Controls.Grid]::SetRow($titlePanel, 0)

$titleText = New-Object System.Windows.Controls.TextBlock
$titleText.Text = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('${heading_b64}'))
$titleText.FontSize = ${S.fontSizes.title}
$titleText.FontWeight = 'Bold'
$titleText.Foreground = '${S.colors.labelText}'
$titleText.FontFamily = '${S.fontFamily.text}'
$titleText.HorizontalAlignment = 'Center'
$titlePanel.AddChild($titleText)
$grid.AddChild($titlePanel)

${generateMetaInfoBar(sessionId, cwd, additionalFields)}

# Content
${lastMessage ? `
$contentFrame = New-Object System.Windows.Controls.Border
$contentFrame.Margin = '16,12,16,12'
$contentFrame.Background = '${S.colors.cardBackground}'
$contentFrame.CornerRadius = '${S.cornerRadius.card}'
$contentFrame.Padding = '16'
${S.cardEffect('contentFrame')}
[System.Windows.Controls.Grid]::SetRow($contentFrame, 2)

$scrollViewer = New-Object System.Windows.Controls.ScrollViewer
$scrollViewer.VerticalScrollBarVisibility = 'Auto'
$scrollViewer.HorizontalScrollBarVisibility = 'Disabled'

$card = New-Object System.Windows.Controls.StackPanel

$msgLbl = New-Object System.Windows.Controls.TextBlock
$msgLbl.Text = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('${msgLabel_b64}'))
$msgLbl.FontSize = ${S.fontSizes.content}
$msgLbl.FontWeight = 'Bold'
$msgLbl.Foreground = '${S.colors.labelText}'
$msgLbl.FontFamily = '${S.fontFamily.text}'
$card.AddChild($msgLbl)

$msg = New-Object System.Windows.Controls.TextBlock
$msg.Text = '${lastMessage.replace(/'/g, "''").substring(0, 500)}'
$msg.FontFamily = '${S.fontFamily.mono}'
$msg.FontSize = ${S.fontSizes.message}
$msg.Foreground = '${S.colors.valueText}'
$msg.Margin = '0,4,0,0'
$msg.TextWrapping = 'Wrap'
$card.AddChild($msg)

$scrollViewer.Content = $card
$contentFrame.Child = $scrollViewer
$grid.AddChild($contentFrame)
` : ''}

# Button
$btnPanel = New-Object System.Windows.Controls.StackPanel
$btnPanel.HorizontalAlignment = 'Center'
$btnPanel.Margin = '16,12,16,16'
[System.Windows.Controls.Grid]::SetRow($btnPanel, 3)

$btn = New-Object System.Windows.Controls.Button
$btn.Content = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('${btnText_b64}'))
$btn.Width = 120
$btn.Height = 36
${S.buttonStyle('btn', 'primary')}
$btn.Add_Click({ $w.Close() })
$btnPanel.AddChild($btn)

$grid.AddChild($btnPanel)

$w.Content = $grid

$w.Add_Loaded({
    $w.Topmost = $true
    $w.Activate()
    $btn.Focus()
    $w.Topmost = $false
})

$w.Add_KeyDown({
    param($sender, $e)
    if ($e.Key -eq 'Escape' -or $e.Key -eq 'Return') { $w.Close() }
})

$w.ShowDialog() | Out-Null
`;

  try {
    executePowerShellScript(psScript);
  } catch (err) {
    logError(`[Stop] Error: ${err.message}\n`);
  }
}

// ============================================
// PostToolUse 事件处理
// ============================================

function handlePostToolUse(data) {
  const toolName = extractField(data, 'tool_name');

  if (toolName === TOOL_ASK_USER_QUESTION) {
    playSound(SOUND_ASTERISK);
  }
}

// ============================================
// 预览模式：注入模拟数据依次渲染三弹窗，供目视验收
// ============================================

function runPreview() {
  const baseData = {
    session_id: 'a1b2c3d4-preview-test',
    cwd: process.cwd()
  };

  console.log('[preview] 1/4 PermissionRequest 权限申请弹窗...');
  handlePermissionRequest({
    ...baseData,
    hook_event_name: EVENT_PERMISSION_REQUEST,
    tool_name: 'Bash',
    tool_input: { command: 'rm -rf /tmp/preview-test', description: '预览模拟操作' },
    permission_suggestions: []
  });

  console.log('[preview] 2/4 AskUserQuestion 单选弹窗...');
  handlePreToolUse({
    ...baseData,
    hook_event_name: EVENT_PRE_TOOL_USE,
    tool_name: TOOL_ASK_USER_QUESTION,
    tool_input: {
      questions: [{
        header: '预览问题',
        question: '这是一个预览问题，请选择一个选项查看单选弹窗效果：',
        multiSelect: false,
        options: [
          { label: '选项一', description: '第一个选项的描述说明' },
          { label: '选项二', description: '第二个选项的描述说明' },
          { label: '选项三', description: '第三个选项的描述说明' }
        ]
      }]
    }
  });

  console.log('[preview] 3/4 AskUserQuestion 多选弹窗...');
  handlePreToolUse({
    ...baseData,
    hook_event_name: EVENT_PRE_TOOL_USE,
    tool_name: TOOL_ASK_USER_QUESTION,
    tool_input: {
      questions: [{
        header: '预览多选',
        question: '这是一个多选预览问题，可勾选多个选项：',
        multiSelect: true,
        options: [
          { label: '多选一', description: '第一个多选项' },
          { label: '多选二', description: '第二个多选项' }
        ]
      }]
    }
  });

  console.log('[preview] 4/4 Stop 任务完成弹窗...');
  handleStop({
    ...baseData,
    hook_event_name: EVENT_STOP,
    last_assistant_message: '这是预览的最后一条助手消息，用于展示 Stop 弹窗的消息卡片渲染效果。包含足够长的文本以测试换行与滚动行为。',
    effort: { level: 'high' },
    permission_mode: 'default',
    background_tasks: [{ id: 't1' }, { id: 't2' }]
  });

  console.log('[preview] 全部弹窗展示完毕');
}

// ============================================
// 主入口
// ============================================

if (PREVIEW_MODE) {
  runPreview();
} else {
  let inputData = '';
  process.stdin.setEncoding('utf8');
  process.stdin.on('data', chunk => inputData += chunk);
  process.stdin.on('end', () => {
    try {
      if (!inputData) {
        logError('[main] No input data received\n');
        return;
      }

      const data = JSON.parse(inputData);
      const event = data.hook_event_name || '';

      if (event === EVENT_PERMISSION_REQUEST) {
        handlePermissionRequest(data);
      } else if (event === EVENT_STOP) {
        handleStop(data);
      } else if (event === EVENT_PRE_TOOL_USE) {
        handlePreToolUse(data);
      } else if (event === EVENT_POST_TOOL_USE) {
        handlePostToolUse(data);
      }
    } catch (err) {
      logError(`[error] ${err.stack}\n`);
    }
  });
}

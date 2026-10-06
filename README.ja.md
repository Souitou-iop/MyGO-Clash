<p align="center">
  <img src="resources/icon.png" width="168" alt="MyGO-Clash">
</p>

<h1 align="center">MyGO-Clash</h1>

<p align="center">
  <a href="https://github.com/MetaCubeX/mihomo">mihomo</a> コアと <a href="https://github.com/egoist/mygo">MyGo</a> フレームワークで作られた、クロスプラットフォームのプロキシクライアント<br>
  <i>迷子でもいい、前へ進め。</i>
</p>

<p align="center">
  <a href="README.md">简体中文</a> · <a href="README.en.md">English</a> · <b>日本語</b>
</p>

<p align="center">
  <img src="https://img.shields.io/badge/platform-macOS%20%7C%20Windows%20%7C%20Linux-4f8fc0" alt="対応プラットフォーム">
  <img src="https://img.shields.io/badge/core-mihomo%201.19.32-e2617f" alt="コア">
  <img src="https://img.shields.io/badge/license-GPL--3.0-2a7ab0" alt="ライセンス">
  <img src="https://img.shields.io/badge/status-preview-e8b84a" alt="ステータス">
</p>

> [!NOTE]
> MyGO-Clash は v0.1.0 の早期プレビュー版です。画面と主な操作は macOS（Apple シリコン）で確認済みですが、TUN とシステムサービス、システムプロキシの切り替え、Tailscale へのログイン、WebDAV 同期、Windows と Linux での動作は、実環境でまだ十分に検証できていません。問題があれば [Issues](https://github.com/Souitou-iop/MyGO-Clash/issues) で教えてください。

## 機能

### プロキシ

- mihomo コアを内蔵し、独立したプロセスとして実行。システムサービスを入れると TUN モードが使えます
- ルール・グローバル・ダイレクトの 3 モード。ホーム、トレイ、クイックパネル、グローバルショートカットから切り替え
- システムプロキシ：除外リスト、PAC スクリプト、ほかのアプリに書き換えられたときの自動復元
- TUN：Mixed・gVisor・System スタック、自動ルート、厳格ルート、DNS ハイジャック、除外する範囲
- ノードの遅延テストと自動チェック。ノードやモードを変えたとき、既存の接続を閉じて新しい経路でつなぎ直せます
- LAN 共有、IPv6、プロセスの判定、Web ダッシュボード用の外部コントローラー

### プロファイル

- リモート購読：使用量と有効期限を表示、定期的に自動更新。直接つながらない購読はプロキシ経由で更新できます
- ローカルのプロファイルと、組み込みの YAML エディター
- 拡張：全体または個別のプロファイルへの YAML マージと JavaScript スクリプト、ルール・ノード・プロキシグループへの先頭追加・末尾追加・削除
- 共有リンク（vmess://、ss://、trojan://、vless://、hysteria2:// など）からノードを取り込み
- `clash://install-config?url=…` リンクからワンクリックで購読を追加

### Tailscale

- ユーザー空間で動く Tailscale ノードを内蔵：ほかのソフトも管理者権限も不要。インストール済みの Tailscale アプリを操作することもできます
- サブネットルートと出口ノードをルールで利用。MagicDNS や、Headscale などのセルフホストの制御サーバーにも対応
- 同じアカウントのほかのデバイスから、Tailscale アドレス経由でこのコンピューターのプロキシを使えます
- TUN モードと Tailscale がぶつからないように調整

### 同期とバックアップ

- 任意の WebDAV サーバー（Nextcloud、坚果云、Synology、InfiniCLOUD など）で、複数のコンピューター間のプロファイルと設定を同期
- エンドツーエンド暗号化：パスフレーズから Argon2id で鍵を導出し、ファイルごとに XChaCha20-Poly1305 で暗号化。ファイル名も隠します
- 同じプロファイルが 2 台で変更されたときは競合として知らせ、設定は項目ごとにマージ。TUN やショートカットなど端末固有の設定は同期しません
- ローカルまたは WebDAV へのバックアップと、暗号化ファイルへのエクスポート

### ツール

- 解除チェック：Netflix、Disney+、YouTube Premium、ChatGPT、Claude、Gemini、Spotify、TikTok、Steam の通貨
- 出口 IP の確認と、主要サイトへの接続チェック
- 接続・ルール・ログの各画面

### デスクトップ

- トレイメニュー（モード、ノード、プロファイル、Tailscale）、トレイの通信速度表示、ネイティブのクイックパネル、グローバルショートカット
- ホームのカードは表示と並び順を自由に変えられ、ウィンドウの幅に合わせて配置されます
- ライト・ダークテーマと複数のアクセントカラー、フォントや CSS のカスタマイズ。表示言語は簡体字中国語と英語
- ウィンドウが前面にないときは、エラーと警告をシステムのネイティブ通知で表示。クリックすると関連する画面が開きます
- 軽量モード：Web ビューを閉じてメモリを節約し、コアとトレイはそのまま動作
- ログイン時の起動、サイレント起動、ターミナル用のプロキシ環境変数のコピー
- 購読 URL やパスワードなどの機密情報は暗号化して保存し、鍵はシステムの安全な保管場所へ（macOS キーチェーン、Windows DPAPI、Linux Secret Service）
- Windows では、Microsoft Store アプリがプロキシを使えるようにワンクリックで設定
- macOS 26 以降では、アプリアイコンがシステムのライト・ダーク・クリア・色合いの外観に合わせて変わります

## ダウンロード

正式版はまだ公開していません。今はソースからビルドしてください（下記）。公開後は [Releases](https://github.com/Souitou-iop/MyGO-Clash/releases) ページからダウンロードできます。

## ソースからのビルド

必要なもの：

- [Go](https://go.dev/dl/) 1.27.1 以降
- [Bun](https://bun.sh)
- Linux：GTK 3 と WebKitGTK 4.1（Debian / Ubuntu なら `sudo apt install libwebkit2gtk-4.1-0`）
- Windows 10：[WebView2 ランタイム](https://developer.microsoft.com/microsoft-edge/webview2/)（Windows 11 には同梱）

```bash
git clone https://github.com/Souitou-iop/MyGO-Clash.git
cd MyGO-Clash
bun install
bun run dev     # 開発モード：変更すると自動で再ビルド
bun run build   # このプラットフォーム向けにパッケージを作成（build/ に出力）
```

ビルドには `package.json` のスクリプトを使ってください。mihomo に必要な `-tags=with_gvisor` が付きます。Go 側の API を変えたら、`bun run generate` でフロントエンドのバインディング `src/mygo.ts` を作り直します。

アプリアイコンは Icon Composer のファイルから作っています。デザインを変えたら、Xcode 26 以降を入れた Mac で次のコマンドを実行すると、macOS 用の `Assets.car`、ほかのプラットフォーム用の `icon.png`、画面内のアイコンがまとめて作り直されます：

```bash
go run ./cmd/genicon path/to/MyGo-Clash.icon
```

## 構成

```
main.go              エントリー：1 つの実行ファイルがアプリ・コア（core）・特権サービス（service）を兼ねる
internal/app         ウィンドウ、トレイ、クイックパネル、通知、設定、フロントエンド向け API
internal/corehost    mihomo コアの実行
internal/coremgr     コアを独立プロセスまたはシステムサービスとして管理
internal/service     TUN に必要な特権サービス
internal/profiles    購読とプロファイル
internal/enhance     マージやスクリプトなどの拡張
internal/tailnet     Tailscale 連携
internal/cloudsync   エンドツーエンド暗号化の同期
internal/sysproxy    システムプロキシ
src/                 React のフロントエンド
cmd/genicon          Icon Composer のファイルから各プラットフォームのアイコンを作成
```

## 謝辞

- [mihomo](https://github.com/MetaCubeX/mihomo)：プロキシコア
- [MyGo](https://github.com/egoist/mygo)：クロスプラットフォームのデスクトップアプリフレームワーク
- [Tailscale](https://tailscale.com)：ネットワーク機能

## 注意事項

- MyGO-Clash は非公式のファンプロジェクトです。名前、配色、コンパスとピックのモチーフは『BanG Dream!』のバンド MyGO!!!!! へのオマージュであり、ブシロードおよびバンド公式とは一切関係ありません。
- 本ソフトウェアはネットワーク技術の学習・研究を目的としています。お住まいの地域の法令を守ってご利用ください。

## ライセンス

[GPL-3.0](LICENSE)

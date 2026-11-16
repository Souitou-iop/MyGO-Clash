// Package tools tests the network through the proxy. This file holds the
// curated preset groups the connectivity page may import at once, after
// MyIP (github.com/jason5ng32/MyIP): what each country reaches at home, and
// the services behind everyday sites abroad. Member names are brand names
// and stay untranslated; group names are locale keys under
// "connectivity.preset.". The data is MyIP's, slightly trimmed.
package tools

import "slices"

// SitePreset is a named group of sites the connectivity page may import.
type SitePreset struct {
	ID    string `json:"id"`
	Emoji string `json:"emoji"`
	Sites []Site `json:"sites"`
}

// SitePresets are the curated groups: country sets, then theme sets. All
// their sites are the page's defaults (ConnectivitySites), shown by
// category.
var SitePresets = []SitePreset{
	{
		ID: "iran", Emoji: "🇮🇷",
		Sites: []Site{
			{"aparat", "Aparat", "https://www.aparat.com/favicon.ico", "global"},
			{"digikala", "Digikala", "https://www.digikala.com/favicon.ico", "global"},
			{"divar", "Divar", "https://divar.ir/favicon.ico", "global"},
			{"cafebazaar", "Cafe Bazaar", "https://cafebazaar.ir/favicon.ico", "global"},
			{"balad", "Balad", "https://balad.ir/favicon.ico", "global"},
			{"filimo", "Filimo", "https://www.filimo.com/favicon.ico", "global"},
			{"torob", "Torob", "https://torob.com/favicon.ico", "global"},
			{"zarinpal", "Zarinpal", "https://www.zarinpal.com/favicon.ico", "global"},
			{"rubika", "Rubika", "https://rubika.ir/favicon.ico", "global"},
			{"eitaa", "Eitaa", "https://eitaa.com/favicon.ico", "global"},
		},
	},
	{
		ID: "russia", Emoji: "🇷🇺",
		Sites: []Site{
			{"yandex", "Yandex", "https://yandex.ru/favicon.ico", "global"},
			{"vk", "VK", "https://vk.com/favicon.ico", "global"},
			{"gosuslugi", "Gosuslugi", "https://www.gosuslugi.ru/favicon.ico", "global"},
			{"ozon", "Ozon", "https://www.ozon.ru/favicon.ico", "global"},
			{"mailru", "Mail.ru", "https://mail.ru/favicon.ico", "global"},
			{"rutube", "Rutube", "https://rutube.ru/favicon.ico", "global"},
			{"wildberries", "Wildberries", "https://www.wildberries.ru/favicon.ico", "global"},
			{"avito", "Avito", "https://www.avito.ru/favicon.ico", "global"},
			{"kinopoisk", "Kinopoisk", "https://www.kinopoisk.ru/favicon.ico", "global"},
			{"2gis", "2GIS", "https://2gis.ru/favicon.ico", "global"},
			{"dzen", "Dzen", "https://dzen.ru/favicon.ico", "global"},
		},
	},
	{
		ID: "china", Emoji: "🇨🇳",
		Sites: []Site{
			{"wechat", "WeChat", "https://res.wx.qq.com/a/wx_fed/assets/res/NTI4MWU5.ico", "domestic"},
			{"baidu", "Baidu", "https://www.baidu.com/favicon.ico", "domestic"},
			{"taobao", "Taobao", "https://www.taobao.com/favicon.ico", "domestic"},
			{"bilibili", "Bilibili", "https://www.bilibili.com/favicon.ico", "domestic"},
			{"qq", "QQ", "https://im.qq.com/favicon.ico", "domestic"},
			{"douyin", "Douyin", "https://www.douyin.com/favicon.ico", "domestic"},
			{"zhihu", "Zhihu", "https://www.zhihu.com/favicon.ico", "domestic"},
			{"jd", "JD", "https://www.jd.com/favicon.ico", "domestic"},
			{"weibo", "Weibo", "https://weibo.com/favicon.ico", "domestic"},
			{"netease", "NetEase", "https://www.163.com/favicon.ico", "domestic"},
			{"xiaohongshu", "Xiaohongshu", "https://www.xiaohongshu.com/favicon.ico", "domestic"},
			{"douban", "Douban", "https://www.douban.com/favicon.ico", "domestic"},
		},
	},
	{
		ID: "brazil", Emoji: "🇧🇷",
		Sites: []Site{
			{"globo", "Globo", "https://www.globo.com/favicon.ico", "global"},
			{"uol", "UOL", "https://www.uol.com.br/favicon.ico", "global"},
			{"govbr", "gov.br", "https://www.gov.br/favicon.ico", "global"},
			{"mercadolivre", "Mercado Livre", "https://www.mercadolivre.com.br/favicon.ico", "global"},
			{"magalu", "Magalu", "https://www.magazineluiza.com.br/favicon.ico", "global"},
			{"americanas", "Americanas", "https://www.americanas.com.br/favicon.ico", "global"},
			{"ifood", "iFood", "https://www.ifood.com.br/favicon.ico", "global"},
			{"olx", "OLX Brasil", "https://www.olx.com.br/favicon.ico", "global"},
			{"nubank", "Nubank", "https://nubank.com.br/favicon.ico", "global"},
			{"picpay", "PicPay", "https://picpay.com/favicon.ico", "global"},
			{"serasa", "Serasa", "https://www.serasa.com.br/favicon.ico", "global"},
			{"reclameaqui", "Reclame Aqui", "https://www.reclameaqui.com.br/favicon.ico", "global"},
		},
	},
	{
		ID: "india", Emoji: "🇮🇳",
		Sites: []Site{
			{"cricbuzz", "Cricbuzz", "https://www.cricbuzz.com/favicon.ico", "global"},
			{"zomato", "Zomato", "https://www.zomato.com/favicon.ico", "global"},
			{"paytm", "Paytm", "https://paytm.com/favicon.ico", "global"},
			{"flipkart", "Flipkart", "https://www.flipkart.com/favicon.ico", "global"},
			{"bigbasket", "BigBasket", "https://www.bigbasket.com/favicon.ico", "global"},
			{"phonepe", "PhonePe", "https://www.phonepe.com/favicon.ico", "global"},
			{"policybazaar", "Policybazaar", "https://www.policybazaar.com/favicon.ico", "global"},
			{"nykaa", "Nykaa", "https://www.nykaa.com/favicon.ico", "global"},
			{"justdial", "Justdial", "https://www.justdial.com/favicon.ico", "global"},
			{"groww", "Groww", "https://groww.in/favicon.ico", "global"},
			{"airtel", "Airtel", "https://www.airtel.in/favicon.ico", "global"},
			{"naukri", "Naukri", "https://www.naukri.com/favicon.ico", "global"},
			{"indigo", "IndiGo", "https://www.goindigo.in/favicon.ico", "global"},
			{"redbus", "RedBus", "https://www.redbus.in/favicon.ico", "global"},
			{"hotstar", "JioHotstar", "https://www.hotstar.com/favicon.ico", "global"},
			{"physicswallah", "Physics Wallah", "https://www.pw.live/favicon.ico", "global"},
		},
	},
	{
		ID: "nigeria", Emoji: "🇳🇬",
		Sites: []Site{
			{"jumia", "Jumia", "https://www.jumia.com.ng/favicon.ico", "global"},
			{"konga", "Konga", "https://www.konga.com/favicon.ico", "global"},
			{"nairaland", "Nairaland", "https://www.nairaland.com/favicon.ico", "global"},
			{"paystack", "Paystack", "https://paystack.com/favicon.ico", "global"},
			{"flutterwave", "Flutterwave", "https://flutterwave.com/favicon.ico", "global"},
			{"zenithbank", "Zenith Bank", "https://www.zenithbank.com/favicon.ico", "global"},
			{"indrive", "inDrive", "https://indrive.com/favicon.ico", "global"},
			{"chowdeck", "Chowdeck", "https://chowdeck.com/favicon.ico", "global"},
			{"mtn", "MTN", "https://www.mtn.ng/favicon.ico", "global"},
			{"boomplay", "Boomplay", "https://boomplay.com/favicon.ico", "global"},
		},
	},
	{
		ID: "germany", Emoji: "🇩🇪",
		Sites: []Site{
			{"deutsche-bahn", "Deutsche Bahn", "https://www.bahn.de/favicon.ico", "global"},
			{"dhl-de", "DHL", "https://www.dhl.de/favicon.ico", "global"},
			{"otto", "OTTO", "https://www.otto.de/favicon.ico", "global"},
			{"spiegel", "DER SPIEGEL", "https://www.spiegel.de/favicon.ico", "global"},
			{"check24", "CHECK24", "https://www.check24.de/favicon.ico", "global"},
			{"bund-de", "Bund.de", "https://www.bund.de/favicon.ico", "global"},
			{"tagesschau", "tagesschau", "https://www.tagesschau.de/favicon.ico", "global"},
			{"kleinanzeigen", "Kleinanzeigen", "https://www.kleinanzeigen.de/favicon.ico", "global"},
			{"zdf", "ZDF", "https://www.zdf.de/favicon.ico", "global"},
			{"telekom-de", "Deutsche Telekom", "https://www.telekom.de/favicon.ico", "global"},
			{"amazon-de", "Amazon.de", "https://www.amazon.de/favicon.ico", "global"},
			{"google-de", "Google.de", "https://www.google.de/favicon.ico", "global"},
		},
	},
	{
		ID: "albania", Emoji: "🇦🇱",
		Sites: []Site{
			{"e-albania", "e-Albania", "https://e-albania.al/favicon.ico", "global"},
			{"tatime", "Tatime", "https://www.tatime.gov.al/favicon.ico", "global"},
			{"balkanweb", "Balkanweb", "https://www.balkanweb.com/favicon.ico", "global"},
			{"a2cnn", "A2 CNN", "https://a2news.com/favicon.ico", "global"},
			{"top-channel", "Top Channel", "https://top-channel.tv/favicon.ico", "global"},
			{"tvklan", "TV Klan", "https://tvklan.al/favicon.ico", "global"},
			{"rtsh", "RTSH", "https://www.rtsh.al/favicon.ico", "global"},
			{"digitalb", "DigitAlb", "https://www.digitalb.al/favicon.ico", "global"},
			{"one-al", "One Albania", "https://www.one.al/favicon.ico", "global"},
			{"vodafone-al", "Vodafone Albania", "https://vodafone.al/favicon.ico", "global"},
			{"credins", "Banka Credins", "https://www.bankacredins.com/favicon.ico", "global"},
			{"intesa-al", "Intesa Sanpaolo Bank", "https://www.intesasanpaolobank.al/.resources/intesaSanPaolo-templating-module/templates/webresources/CIB/images/favicon-32x32.png", "global"},
			{"raiffeisen-al", "Raiffeisen Bank", "https://www.raiffeisen.al/favicon.ico", "global"},
			{"merrjep-al", "MerrJep", "https://www.merrjep.al/favicon.ico", "global"},
			{"oshee", "OSHEE", "https://oshee.al/favicon.ico", "global"},
		},
	},
	{
		ID: "kosovo", Emoji: "🇽🇰",
		Sites: []Site{
			{"ekosova", "e-Kosova", "https://ekosova.rks-gov.net/favicon.ico", "global"},
			{"atk-ks", "ATK", "https://www.atk-ks.org/favicon.ico", "global"},
			{"kosovajob", "KosovaJob", "https://kosovajob.com/favicon.ico", "global"},
			{"gjirafamall", "GjirafaMall", "https://gjirafamall.com/favicon.ico", "global"},
			{"kallxo", "Kallxo", "https://kallxo.com/favicon.ico", "global"},
			{"rtk", "RTK", "https://www.rtklive.com/favicon.ico", "global"},
			{"ipko", "IPKO", "https://www.ipko.com/favicon.ico", "global"},
			{"vala", "Vala", "https://www.kosovotelecom.com/favicon.ico", "global"},
			{"artmotion", "ArtMotion", "https://artmotion.net/favicon.ico", "global"},
			{"procredit-ks", "ProCredit Bank", "https://www.procreditbank-kos.com/favicon.ico", "global"},
			{"raiffeisen-ks", "Raiffeisen Bank", "https://www.raiffeisen-kosovo.com/favicon.ico", "global"},
			{"gjirafa", "Gjirafa", "https://gjirafa.com/favicon.ico", "global"},
			{"wolt", "Wolt", "https://wolt-com-static-assets.wolt.com/favicon.ico", "global"},
			{"merrjep-xk", "MerrJep", "https://www.merrjep.com/favicon.ico", "global"},
			{"keds", "KEDS", "https://www.keds-energy.com/favicon.ico", "global"},
		},
	},
	{
		ID: "thailand", Emoji: "🇹🇭",
		Sites: []Site{
			{"pantip", "Pantip", "https://pantip.com/fav-icon.ico", "global"},
			{"sanook", "Sanook", "https://www.sanook.com/favicon.ico", "global"},
			{"thairath", "Thairath", "https://www.thairath.co.th/static/img/favicon/favicon-32x32.png", "global"},
			{"thaipbs", "Thai PBS", "https://www.thaipbs.or.th/favicon.ico", "global"},
			{"ch3plus", "CH3Plus", "https://www.ch3plus.com/favicon.ico", "global"},
			{"shopee-th", "Shopee", "https://shopee.co.th/favicon.ico", "global"},
			{"lazada-th", "Lazada", "https://www.lazada.co.th/favicon.ico", "global"},
			{"wongnai", "Wongnai", "https://www.wongnai.com/favicon.ico", "global"},
			{"kbank", "KBank", "https://www.kasikornbank.com/favicon.ico", "global"},
			{"scb", "SCB", "https://www.scb.co.th/favicon.ico", "global"},
			{"bangkokbank", "Bangkok Bank", "https://www.bangkokbank.com/favicon.ico", "global"},
			{"ais", "AIS", "https://www.ais.th/favicon.ico", "global"},
			{"true-th", "True", "https://www.true.th/favicon.ico", "global"},
			{"rd-go-th", "Revenue Department", "https://www.rd.go.th/typo3conf/ext/rdinter_theme13/Resources/Public/Icons/favicon-32x32.png", "global"},
			{"mea", "MEA", "https://www.mea.or.th/favicon.ico", "global"},
			{"pea", "PEA", "https://www.pea.co.th/favicon.ico", "global"},
		},
	},
	{
		ID: "mexico", Emoji: "🇲🇽",
		Sites: []Site{
			{"google-mx", "Google México", "https://www.google.com.mx/favicon.ico", "global"},
			{"amazon-mx", "Amazon México", "https://www.amazon.com.mx/favicon.ico", "global"},
			{"mercadolibre-mx", "Mercado Libre", "https://www.mercadolibre.com.mx/favicon.ico", "global"},
			{"walmart-mx", "Walmart México", "https://www.walmart.com.mx/favicon.ico", "global"},
			{"gob-mx", "Gob.mx", "https://framework-gb.cdn.gob.mx/gm/v3/assets/images/favicon.ico", "global"},
			{"imss", "IMSS", "https://www.imss.gob.mx/favicon.ico", "global"},
			{"telcel", "Telcel", "https://www.telcel.com/favicon.ico", "global"},
			{"telmex", "Telmex", "https://telmex.com/favicon.ico", "global"},
			{"totalplay", "Totalplay", "https://www.totalplay.com.mx/favicon.ico", "global"},
			{"bbva-mx", "BBVA México", "https://www.bbva.mx/favicon.ico", "global"},
			{"banorte", "Banorte", "https://www.banorte.com/favicon.ico", "global"},
			{"el-universal", "El Universal", "https://www.eluniversal.com.mx/favicon.ico", "global"},
			{"milenio", "Milenio", "https://www.milenio.com/favicon.ico", "global"},
			{"liverpool-mx", "Liverpool", "https://www.liverpool.com.mx/favicon.ico", "global"},
			{"unam", "UNAM", "https://www.unam.mx/favicon.ico", "global"},
		},
	},
	{
		ID: "japan", Emoji: "🇯🇵",
		Sites: []Site{
			{"yahoo-jp", "Yahoo! JAPAN", "https://www.yahoo.co.jp/favicon.ico", "global"},
			{"nhk", "NHK", "https://www.nhk.or.jp/favicon.ico", "global"},
			{"mercari", "Mercari", "https://jp.mercari.com/favicon.ico", "global"},
			{"nicovideo", "niconico", "https://www.nicovideo.jp/favicon.ico", "global"},
			{"pixiv", "pixiv", "https://www.pixiv.net/favicon.ico", "global"},
			{"abema", "ABEMA", "https://abema.tv/favicon.ico", "global"},
			{"tver", "TVer", "https://tver.jp/favicon.ico", "global"},
			{"dmm-tv", "DMM TV", "https://tv.dmm.com/favicon.ico", "global"},
			{"radiko", "radiko", "https://radiko.jp/area", "global"},
			{"kakaku", "kakaku.com", "https://kakaku.com/favicon.ico", "global"},
			{"tabelog", "Tabelog", "https://tabelog.com/favicon.ico", "global"},
			{"note", "note", "https://note.com/favicon.ico", "global"},
			{"qiita", "Qiita", "https://qiita.com/robots.txt", "global"},
			{"zenn", "Zenn", "https://zenn.dev/favicon.ico", "global"},
			{"nikkei", "Nikkei", "https://www.nikkei.com/favicon.ico", "global"},
			{"asahi", "Asahi Shimbun", "https://www.asahi.com/favicon.ico", "global"},
			{"docomo", "NTT docomo", "https://www.docomo.ne.jp/favicon.ico", "global"},
			{"paypay", "PayPay", "https://paypay.ne.jp/robots.txt", "global"},
			{"jal", "JAL", "https://www.jal.co.jp/favicon.ico", "global"},
			{"uniqlo", "UNIQLO", "https://www.uniqlo.com/favicon.ico", "global"},
		},
	},
	{
		ID: "korea", Emoji: "🇰🇷",
		Sites: []Site{
			{"naver", "Naver", "https://www.naver.com/favicon.ico", "global"},
			{"daum", "Daum", "https://www.daum.net/favicon.ico", "global"},
			{"kakao", "Kakao", "https://www.kakao.com/robots.txt", "global"},
			{"coupang", "Coupang", "https://www.coupang.com/robots.txt", "global"},
			{"melon", "Melon", "https://www.melon.com/favicon.ico", "global"},
			{"tving", "TVING", "https://www.tving.com/favicon.ico", "global"},
			{"wavve", "Wavve", "https://www.wavve.com/favicon.ico", "global"},
			{"coupangplay", "Coupang Play", "https://www.coupangplay.com/favicon.ico", "global"},
			{"naver-webtoon", "Naver Webtoon", "https://comic.naver.com/robots.txt", "global"},
			{"toss", "Toss", "https://toss.im/robots.txt", "global"},
			{"kbs", "KBS", "https://www.kbs.co.kr/robots.txt", "global"},
			{"nexon", "Nexon", "https://www.nexon.com/favicon.ico", "global"},
			{"upbit", "Upbit", "https://upbit.com/favicon.ico", "global"},
			{"dcinside", "DCinside", "https://www.dcinside.com/favicon.ico", "global"},
			{"namuwiki", "Namuwiki", "https://namu.wiki/favicon.ico", "global"},
		},
	},
	{
		ID: "hongkong", Emoji: "🇭🇰",
		Sites: []Site{
			{"hk01", "HK01", "https://www.hk01.com/favicon.ico", "global"},
			{"scmp", "SCMP", "https://www.scmp.com/favicon.ico", "global"},
			{"rthk", "RTHK", "https://www.rthk.hk/favicon.ico", "global"},
			{"hkjc", "HKJC", "https://www.hkjc.com/favicon.ico", "global"},
			{"hsbc-hk", "HSBC HK", "https://www.hsbc.com.hk/robots.txt", "global"},
			{"openrice", "OpenRice", "https://www.openrice.com/favicon.ico", "global"},
			{"hktvmall", "HKTVmall", "https://www.hktvmall.com/favicon.ico", "global"},
			{"mytvsuper", "myTV SUPER", "https://www.mytvsuper.com/favicon.ico", "global"},
			{"viu", "Viu", "https://www.viu.com/favicon.ico", "global"},
			{"tvb", "TVB", "https://www.tvb.com/favicon.ico", "global"},
			{"hkex", "HKEX", "https://www.hkex.com.hk/robots.txt", "global"},
			{"lihkg", "LIHKG", "https://lihkg.com/favicon.ico", "global"},
			{"price-hk", "Price.com.hk", "https://www.price.com.hk/favicon.ico", "global"},
			{"hkt", "HKT", "https://www.hkt.com/favicon.ico", "global"},
		},
	},
	{
		ID: "taiwan", Emoji: "🇹🇼",
		Sites: []Site{
			{"dcard", "Dcard", "https://www.dcard.tw/favicon.ico", "global"},
			{"bahamut", "巴哈姆特", "https://www.gamer.com.tw/robots.txt", "global"},
			{"ltn", "自由时报", "https://www.ltn.com.tw/favicon.ico", "global"},
			{"udn", "UDN", "https://udn.com/favicon.ico", "global"},
			{"setn", "三立新闻", "https://www.setn.com/favicon.ico", "global"},
			{"ettoday", "ETtoday", "https://www.ettoday.net/favicon.ico", "global"},
			{"esun", "玉山银行", "https://www.esunbank.com/favicon.ico", "global"},
			{"591", "591", "https://www.591.com.tw/favicon.ico", "global"},
			{"yahoo-tw", "Yahoo 奇摩", "https://tw.yahoo.com/favicon.ico", "global"},
			{"cna", "中央社", "https://www.cna.com.tw/favicon.ico", "global"},
			{"shopee-tw", "蝦皮", "https://shopee.tw/favicon.ico", "global"},
			{"kktv", "KKTV", "https://www.kktv.me/favicon.ico", "global"},
			{"litv", "LiTV", "https://www.litv.tv/favicon.ico", "global"},
			{"hami", "Hami Video", "https://hamivideo.hinet.net/favicon.ico", "global"},
		},
	},
	{
		ID: "singapore", Emoji: "🇸🇬",
		Sites: []Site{
			{"straitstimes", "The Straits Times", "https://www.straitstimes.com/favicon.ico", "global"},
			{"cna-sg", "CNA", "https://www.channelnewsasia.com/favicon.ico", "global"},
			{"grab", "Grab", "https://www.grab.com/favicon.ico", "global"},
			{"dbs", "DBS", "https://www.dbs.com.sg/favicon.ico", "global"},
			{"uob", "UOB", "https://www.uob.com.sg/favicon.ico", "global"},
			{"singtel", "Singtel", "https://www.singtel.com/robots.txt", "global"},
			{"gov-sg", "gov.sg", "https://www.gov.sg/robots.txt", "global"},
			{"singpass", "Singpass", "https://www.singpass.gov.sg/favicon.ico", "global"},
			{"mediacorp", "Mediacorp", "https://www.mediacorp.sg/favicon.ico", "global"},
			{"carousell", "Carousell", "https://www.carousell.sg/robots.txt", "global"},
		},
	},
	{
		ID: "usa", Emoji: "🇺🇸",
		Sites: []Site{
			{"craigslist", "Craigslist", "https://www.craigslist.org/favicon.ico", "global"},
			{"yelp", "Yelp", "https://www.yelp.com/favicon.ico", "global"},
			{"imdb", "IMDb", "https://www.imdb.com/favicon.ico", "global"},
			{"weather", "The Weather Channel", "https://weather.com/favicon.ico", "global"},
			{"irs", "IRS", "https://www.irs.gov/favicon.ico", "global"},
			{"usps", "USPS", "https://www.usps.com/favicon.ico", "global"},
			{"ups", "UPS", "https://www.ups.com/favicon.ico", "global"},
			{"fedex", "FedEx", "https://www.fedex.com/favicon.ico", "global"},
			{"target", "Target", "https://www.target.com/favicon.ico", "global"},
			{"costco", "Costco", "https://www.costco.com/favicon.ico", "global"},
			{"bestbuy", "Best Buy", "https://www.bestbuy.com/favicon.ico", "global"},
			{"chase", "Chase", "https://www.chase.com/favicon.ico", "global"},
			{"wellsfargo", "Wells Fargo", "https://www.wellsfargo.com/favicon.ico", "global"},
			{"bankofamerica", "Bank of America", "https://www.bankofamerica.com/favicon.ico", "global"},
			{"usatoday", "USA Today", "https://www.usatoday.com/favicon.ico", "global"},
		},
	},
	{
		ID: "uk", Emoji: "🇬🇧",
		Sites: []Site{
			{"govuk", "GOV.UK", "https://www.gov.uk/favicon.ico", "global"},
			{"nhs", "NHS", "https://www.nhs.uk/robots.txt", "global"},
			{"channel4", "Channel 4", "https://www.channel4.com/favicon.ico", "global"},
			{"sky", "Sky", "https://www.sky.com/favicon.ico", "global"},
			{"tesco", "Tesco", "https://www.tesco.com/favicon.ico", "global"},
			{"argos", "Argos", "https://www.argos.co.uk/favicon.ico", "global"},
			{"lloyds", "Lloyds Bank", "https://www.lloydsbank.com/robots.txt", "global"},
			{"barclays", "Barclays", "https://www.barclays.co.uk/robots.txt", "global"},
			{"hsbc-uk", "HSBC UK", "https://www.hsbc.co.uk/robots.txt", "global"},
			{"rightmove", "Rightmove", "https://www.rightmove.co.uk/favicon.ico", "global"},
			{"gumtree", "Gumtree", "https://www.gumtree.com/favicon.ico", "global"},
			{"telegraph", "The Telegraph", "https://www.telegraph.co.uk/favicon.ico", "global"},
			{"dailymail", "Daily Mail", "https://www.dailymail.co.uk/favicon.ico", "global"},
			{"skynews", "Sky News", "https://news.sky.com/favicon.ico", "global"},
			{"bt", "BT", "https://www.bt.com/robots.txt", "global"},
			{"vodafone-uk", "Vodafone UK", "https://www.vodafone.co.uk/favicon.ico", "global"},
		},
	},
	{
		// The web's front doors, each at the address its maker's own
		// devices check the connection with, where it has one.
		ID: "web", Emoji: "🌐",
		Sites: []Site{
			{"google", "Google", "https://www.google.com/generate_204", "global"},
			{"apple", "Apple", "https://www.apple.com/library/test/success.html", "global"},
			{"microsoft", "Microsoft", "https://www.msftconnecttest.com/connecttest.txt", "global"},
			{"duckduckgo", "DuckDuckGo", "https://duckduckgo.com/favicon.ico", "global"},
			{"bing", "Bing", "https://www.bing.com/favicon.ico", "global"},
			{"yahoo", "Yahoo", "https://www.yahoo.com/favicon.ico", "global"},
			{"brave", "Brave", "https://brave.com/favicon.ico", "global"},
			{"ecosia", "Ecosia", "https://www.ecosia.org/favicon.ico", "global"},
			{"cloudflare-dns", "1.1.1.1", "https://1.1.1.1/cdn-cgi/trace", "global"},
			{"google-dns", "Google DNS", "https://dns.google/", "global"},
			{"google-fonts", "Google Fonts", "https://fonts.googleapis.com/css?family=Roboto", "global"},
			{"google-maps", "Google Maps", "https://maps.google.com/", "global"},
			{"archive", "Internet Archive", "https://archive.org/favicon.ico", "global"},
		},
	},
	{
		ID: "ai", Emoji: "🤖",
		Sites: []Site{
			{"claude", "Claude", "https://claude.com/favicon.ico", "global"},
			{"chatgpt", "ChatGPT", "https://chatgpt.com/favicon.ico", "global"},
			{"gemini", "Gemini", "https://gemini.google.com/favicon.ico", "global"},
			{"deepseek", "DeepSeek", "https://chat.deepseek.com/favicon.ico", "global"},
			{"huggingface", "Hugging Face", "https://huggingface.co/favicon.ico", "global"},
			{"midjourney", "Midjourney", "https://www.midjourney.com/favicon.ico", "global"},
			{"suno", "Suno", "https://suno.com/favicon.ico", "global"},
			{"manus", "Manus", "https://manus.im/favicon.ico", "global"},
			{"mistral", "Mistral", "https://chat.mistral.ai/favicon.ico", "global"},
			{"meta-ai", "Meta AI", "https://www.meta.ai/favicon.ico", "global"},
			{"qwen", "Qwen", "https://chat.qwen.ai/favicon.ico", "global"},
			{"kimi", "Kimi", "https://www.kimi.com/favicon.ico", "global"},
			{"poe", "Poe", "https://poe.com/favicon.ico", "global"},
			{"characterai", "Character.ai", "https://character.ai/favicon.ico", "global"},
			{"perplexity", "Perplexity", "https://www.perplexity.ai/favicon.ico", "global"},
			{"grok", "Grok", "https://grok.com/robots.txt", "global"},
			{"copilot", "Copilot", "https://copilot.microsoft.com/robots.txt", "global"},
			{"cursor", "Cursor", "https://cursor.com/favicon.ico", "global"},
			{"openrouter", "OpenRouter", "https://openrouter.ai/favicon.ico", "global"},
			{"elevenlabs", "ElevenLabs", "https://elevenlabs.io/favicon.ico", "global"},
			{"groq", "Groq", "https://groq.com/favicon.ico", "global"},
			{"cohere", "Cohere", "https://cohere.com/favicon.ico", "global"},
			{"ollama", "Ollama", "https://ollama.com/robots.txt", "global"},
			{"replicate", "Replicate", "https://replicate.com/robots.txt", "global"},
			{"civitai", "Civitai", "https://civitai.com/favicon.ico", "global"},
			{"notebooklm", "NotebookLM", "https://notebooklm.google.com/favicon.ico", "global"},
			{"aistudio", "Google AI Studio", "https://aistudio.google.com/welcome", "global"},
			{"openai-api", "OpenAI API", "https://api.openai.com/robots.txt", "global"},
			{"anthropic-api", "Anthropic API", "https://api.anthropic.com/robots.txt", "global"},
			{"gemini-api", "Gemini API", "https://generativelanguage.googleapis.com/robots.txt", "global"},
		},
	},
	{
		ID: "social", Emoji: "💬",
		Sites: []Site{
			{"telegram", "Telegram", "https://telegram.org/favicon.ico", "global"},
			{"whatsapp", "WhatsApp", "https://whatsapp.com/favicon.ico", "global"},
			{"instagram", "Instagram", "https://www.instagram.com/favicon.ico", "global"},
			{"x", "X", "https://x.com/favicon.ico", "global"},
			{"discord", "Discord", "https://discord.com/favicon.ico", "global"},
			{"signal", "Signal", "https://signal.org/favicon.ico", "global"},
			{"facebook", "Facebook", "https://www.facebook.com/favicon.ico", "global"},
			{"reddit", "Reddit", "https://www.reddit.com/favicon.ico", "global"},
			{"threads", "Threads", "https://www.threads.com/favicon.ico", "global"},
			{"line", "LINE", "https://line.me/favicon.ico", "global"},
			{"viber", "Viber", "https://www.viber.com/favicon.ico", "global"},
			{"snapchat", "Snapchat", "https://www.snapchat.com/favicon.ico", "global"},
			{"pinterest", "Pinterest", "https://www.pinterest.com/favicon.ico", "global"},
			{"linkedin", "LinkedIn", "https://www.linkedin.com/favicon.ico", "global"},
			{"tumblr", "Tumblr", "https://www.tumblr.com/favicon.ico", "global"},
			{"bluesky", "Bluesky", "https://bsky.app/robots.txt", "global"},
			{"mastodon", "Mastodon", "https://mastodon.social/robots.txt", "global"},
			{"medium", "Medium", "https://medium.com/favicon.ico", "global"},
			{"quora", "Quora", "https://www.quora.com/favicon.ico", "global"},
			{"imgur", "Imgur", "https://imgur.com/favicon.ico", "global"},
			{"substack", "Substack", "https://substack.com/favicon.ico", "global"},
			{"patreon", "Patreon", "https://www.patreon.com/favicon.ico", "global"},
		},
	},
	{
		ID: "productivity", Emoji: "💼",
		Sites: []Site{
			{"notion", "Notion", "https://www.notion.so/front-static/favicon.ico", "global"},
			{"slack", "Slack", "https://slack.com/favicon.ico", "global"},
			{"zoom", "Zoom", "https://zoom.us/favicon.ico", "global"},
			{"figma", "Figma", "https://static.figma.com/app/icon/2/favicon.ico", "global"},
			{"trello", "Trello", "https://trello.com/favicon.ico", "global"},
			{"asana", "Asana", "https://asana.com/favicon.ico", "global"},
			{"dropbox", "Dropbox", "https://cfl.dropboxstatic.com/static/metaserver/static/images/favicon.ico", "global"},
			{"canva", "Canva", "https://www.canva.com/favicon.ico", "global"},
			{"miro", "Miro", "https://miro.com/favicon.ico", "global"},
			{"airtable", "Airtable", "https://airtable.com/favicon.ico", "global"},
			{"linear", "Linear", "https://linear.app/favicon.ico", "global"},
			{"clickup", "ClickUp", "https://clickup.com/favicon.ico", "global"},
			{"gmail", "Gmail", "https://mail.google.com/favicon.ico", "global"},
			{"google-drive", "Google Drive", "https://drive.google.com/favicon.ico", "global"},
			{"outlook", "Outlook", "https://outlook.live.com/favicon.ico", "global"},
			{"onedrive", "OneDrive", "https://onedrive.live.com/favicon.ico", "global"},
			{"icloud", "iCloud", "https://www.icloud.com/favicon.ico", "global"},
			{"proton", "Proton", "https://proton.me/favicon.ico", "global"},
			{"colab", "Google Colab", "https://colab.research.google.com/favicon.ico", "global"},
			{"kaggle", "Kaggle", "https://www.kaggle.com/favicon.ico", "global"},
		},
	},
	{
		ID: "education", Emoji: "🎓",
		Sites: []Site{
			{"coursera", "Coursera", "https://www.coursera.org/favicon.ico", "global"},
			{"edx", "edX", "https://www.edx.org/favicon.ico", "global"},
			{"khan-academy", "Khan Academy", "https://www.khanacademy.org/favicon.ico", "global"},
			{"udemy", "Udemy", "https://www.udemy.com/staticx/udemy/images/v8/favicon-32x32.png", "global"},
			{"duolingo", "Duolingo", "https://www.duolingo.com/robots.txt", "global"},
			{"codecademy", "Codecademy", "https://www.codecademy.com/favicon.ico", "global"},
			{"brilliant", "Brilliant", "https://brilliant.org/favicon.ico", "global"},
			{"skillshare", "Skillshare", "https://www.skillshare.com/favicon.ico", "global"},
			{"futurelearn", "FutureLearn", "https://www.futurelearn.com/favicon.ico", "global"},
			{"masterclass", "MasterClass", "https://www.masterclass.com/favicon-32x32.png", "global"},
			{"leetcode", "LeetCode", "https://leetcode.com/favicon.ico", "global"},
			{"mit-ocw", "MIT OpenCourseWare", "https://ocw.mit.edu/favicon.ico", "global"},
		},
	},
	{
		ID: "streaming", Emoji: "🎬",
		Sites: []Site{
			{"youtube", "YouTube", "https://www.youtube.com/favicon.ico", "global"},
			{"netflix", "Netflix", "https://www.netflix.com/favicon.ico", "global"},
			{"spotify", "Spotify", "https://open.spotify.com/favicon.ico", "global"},
			{"tiktok", "TikTok", "https://www.tiktok.com/favicon.ico", "global"},
			{"twitch", "Twitch", "https://www.twitch.tv/favicon.ico", "global"},
			{"primevideo", "Prime Video", "https://www.primevideo.com/favicon.ico", "global"},
			{"disneyplus", "Disney+", "https://www.disneyplus.com/favicon.ico", "global"},
			{"hbomax", "HBO Max", "https://www.hbomax.com/favicon.ico", "global"},
			{"appletv", "Apple TV+", "https://tv.apple.com/favicon.ico", "global"},
			{"soundcloud", "SoundCloud", "https://soundcloud.com/favicon.ico", "global"},
			{"vimeo", "Vimeo", "https://vimeo.com/favicon.ico", "global"},
			{"dailymotion", "Dailymotion", "https://www.dailymotion.com/favicon.ico", "global"},
			{"hulu", "Hulu", "https://www.hulu.com/favicon.ico", "global"},
			{"peacock", "Peacock", "https://www.peacocktv.com/favicon.ico", "global"},
			{"paramount", "Paramount+", "https://www.paramountplus.com/favicon.ico", "global"},
			{"crunchyroll", "Crunchyroll", "https://www.crunchyroll.com/favicon.ico", "global"},
			{"plex", "Plex", "https://www.plex.tv/favicon.ico", "global"},
			{"tubi", "Tubi", "https://tubitv.com/favicon.ico", "global"},
			{"pluto", "Pluto TV", "https://pluto.tv/favicon.ico", "global"},
			{"viki", "Rakuten Viki", "https://www.viki.com/favicon.ico", "global"},
			{"bilibili-tv", "Bilibili Global", "https://www.bilibili.tv/favicon.ico", "global"},
			{"iqiyi-intl", "iQIYI Intl", "https://www.iq.com/favicon.ico", "global"},
			{"wetv", "WeTV", "https://wetv.vip/favicon.ico", "global"},
			{"kick", "Kick", "https://kick.com/favicon.ico", "global"},
			{"kkbox", "KKBOX", "https://www.kkbox.com/favicon.ico", "global"},
			{"joox", "JOOX", "https://www.joox.com/favicon.ico", "global"},
		},
	},
	{
		ID: "music", Emoji: "🎵",
		Sites: []Site{
			{"tidal", "TIDAL", "https://tidal.com/favicon.ico", "global"},
			{"deezer", "Deezer", "https://www.deezer.com/favicon.ico", "global"},
			{"tunein", "TuneIn", "https://tunein.com/favicon.ico", "global"},
			{"youtube-music", "YouTube Music", "https://music.youtube.com/favicon.ico", "global"},
			{"pandora", "Pandora", "https://www.pandora.com/favicon.ico", "global"},
			{"lastfm", "Last.fm", "https://www.last.fm/favicon.ico", "global"},
			{"audiomack", "Audiomack", "https://audiomack.com/favicon.ico", "global"},
			{"musixmatch", "Musixmatch", "https://www.musixmatch.com/favicon.ico", "global"},
			{"apple-music", "Apple Music", "https://music.apple.com/favicon.ico", "global"},
			{"mixcloud", "Mixcloud", "https://www.mixcloud.com/favicon.ico", "global"},
			{"qobuz", "Qobuz", "https://www.qobuz.com/favicon.ico", "global"},
			{"iheart", "iHeartRadio", "https://www.iheart.com/favicon.ico", "global"},
		},
	},
	{
		ID: "sports", Emoji: "🏆",
		Sites: []Site{
			{"espn", "ESPN", "https://www.espn.com/favicon.ico", "global"},
			{"nba", "NBA", "https://www.nba.com/favicon.ico", "global"},
			{"fifa", "FIFA", "https://www.fifa.com/favicon.ico", "global"},
			{"premierleague", "Premier League", "https://www.premierleague.com/favicon.ico", "global"},
			{"formula1", "Formula 1", "https://www.formula1.com/favicon.ico", "global"},
			{"dazn", "DAZN", "https://www.dazn.com/favicon.ico", "global"},
			{"flashscore", "Flashscore", "https://www.flashscore.com/favicon.ico", "global"},
			{"sofascore", "Sofascore", "https://www.sofascore.com/favicon.ico", "global"},
			{"skysports", "Sky Sports", "https://www.skysports.com/favicon.ico", "global"},
			{"bein-sports", "beIN Sports", "https://www.beinsports.com/favicon.ico", "global"},
			{"mlb", "MLB", "https://www.mlb.com/favicon.ico", "global"},
			{"nfl", "NFL", "https://www.nfl.com/favicon.ico", "global"},
			{"uefa", "UEFA", "https://www.uefa.com/favicon.ico", "global"},
			{"laliga", "LaLiga", "https://www.laliga.com/favicon.ico", "global"},
			{"bundesliga", "Bundesliga", "https://www.bundesliga.com/favicon.ico", "global"},
			{"ufc", "UFC", "https://www.ufc.com/favicon.ico", "global"},
			{"wwe", "WWE", "https://www.wwe.com/favicon.ico", "global"},
			{"olympics", "Olympics", "https://olympics.com/favicon.ico", "global"},
			{"transfermarkt", "Transfermarkt", "https://www.transfermarkt.com/favicon.ico", "global"},
			{"fotmob", "FotMob", "https://www.fotmob.com/favicon.ico", "global"},
			{"strava", "Strava", "https://www.strava.com/favicon.ico", "global"},
			{"goal", "Goal", "https://www.goal.com/favicon.ico", "global"},
		},
	},
	{
		ID: "travel", Emoji: "✈️",
		Sites: []Site{
			{"booking", "Booking.com", "https://www.booking.com/favicon.ico", "global"},
			{"airbnb", "Airbnb", "https://www.airbnb.com/favicon.ico", "global"},
			{"expedia", "Expedia", "https://www.expedia.com/favicon.ico", "global"},
			{"skyscanner", "Skyscanner", "https://www.skyscanner.net/favicon.ico", "global"},
			{"tripadvisor", "Tripadvisor", "https://www.tripadvisor.com/favicon.ico", "global"},
			{"agoda", "Agoda", "https://www.agoda.com/favicon.ico", "global"},
			{"kayak", "KAYAK", "https://www.kayak.com/favicon.ico", "global"},
			{"trip-com", "Trip.com", "https://www.trip.com/favicon.ico", "global"},
			{"hotels-com", "Hotels.com", "https://www.hotels.com/favicon.ico", "global"},
			{"makemytrip", "MakeMyTrip", "https://www.makemytrip.com/favicon.ico", "global"},
			{"uber", "Uber", "https://www.uber.com/favicon.ico", "global"},
			{"lyft", "Lyft", "https://www.lyft.com/favicon.ico", "global"},
			{"openstreetmap", "OpenStreetMap", "https://www.openstreetmap.org/favicon.ico", "global"},
			{"emirates", "Emirates", "https://www.emirates.com/favicon.ico", "global"},
			{"singaporeair", "Singapore Airlines", "https://www.singaporeair.com/favicon.ico", "global"},
			{"cathaypacific", "Cathay Pacific", "https://www.cathaypacific.com/favicon.ico", "global"},
			{"delta", "Delta", "https://www.delta.com/favicon.ico", "global"},
			{"united", "United Airlines", "https://www.united.com/favicon.ico", "global"},
			{"lufthansa", "Lufthansa", "https://www.lufthansa.com/favicon.ico", "global"},
			{"ryanair", "Ryanair", "https://www.ryanair.com/favicon.ico", "global"},
			{"hostelworld", "Hostelworld", "https://www.hostelworld.com/favicon.ico", "global"},
			{"airalo", "Airalo", "https://www.airalo.com/favicon.ico", "global"},
		},
	},
	{
		ID: "gaming", Emoji: "🎮",
		Sites: []Site{
			{"steam", "Steam", "https://store.steampowered.com/favicon.ico", "global"},
			{"epicgames", "Epic Games", "https://store.epicgames.com/favicon.ico", "global"},
			{"playstation", "PlayStation", "https://www.playstation.com/favicon.ico", "global"},
			{"xbox", "Xbox", "https://www.xbox.com/favicon.ico", "global"},
			{"riotgames", "Riot Games", "https://www.riotgames.com/favicon.ico", "global"},
			{"nintendo", "Nintendo", "https://www.nintendo.com/favicon.ico", "global"},
			{"ea", "EA", "https://www.ea.com/favicon.ico", "global"},
			{"ubisoft", "Ubisoft", "https://www.ubisoft.com/favicon.ico", "global"},
			{"battlenet", "Battle.net", "https://www.blizzard.com/favicon.ico", "global"},
			{"rockstar", "Rockstar", "https://www.rockstargames.com/favicon.ico", "global"},
			{"roblox", "Roblox", "https://www.roblox.com/favicon.ico", "global"},
			{"itchio", "itch.io", "https://itch.io/favicon.ico", "global"},
			{"steamcommunity", "Steam Community", "https://steamcommunity.com/favicon.ico", "global"},
			{"gog", "GOG", "https://www.gog.com/favicon.ico", "global"},
			{"hoyoverse", "HoYoverse", "https://www.hoyoverse.com/favicon.ico", "global"},
			{"minecraft", "Minecraft", "https://www.minecraft.net/favicon.ico", "global"},
			{"supercell", "Supercell", "https://supercell.com/favicon.ico", "global"},
			{"humble", "Humble Bundle", "https://www.humblebundle.com/favicon.ico", "global"},
			{"garena", "Garena", "https://www.garena.com/favicon.ico", "global"},
			{"psn", "PlayStation Store", "https://store.playstation.com/favicon.ico", "global"},
			{"appstore", "App Store", "https://apps.apple.com/favicon.ico", "global"},
		},
	},
	{
		ID: "developer", Emoji: "👨‍💻",
		Sites: []Site{
			{"github", "GitHub", "https://github.com/favicon.ico", "global"},
			{"npm", "npm", "https://registry.npmjs.org/favicon.ico", "global"},
			{"dockerhub", "Docker Hub", "https://hub.docker.com/favicon.ico", "global"},
			{"stackoverflow", "Stack Overflow", "https://stackoverflow.com/favicon.ico", "global"},
			{"pypi", "PyPI", "https://pypi.org/favicon.ico", "global"},
			{"gitlab", "GitLab", "https://gitlab.com/favicon.ico", "global"},
			{"bitbucket", "Bitbucket", "https://bitbucket.org/favicon.ico", "global"},
			{"crates", "crates.io", "https://crates.io/favicon.ico", "global"},
			{"maven", "Maven Central", "https://repo.maven.apache.org/favicon.ico", "global"},
			{"goproxy", "Go Proxy", "https://proxy.golang.org/favicon.ico", "global"},
			{"homebrew", "Homebrew", "https://brew.sh/favicon.ico", "global"},
			{"jetbrains", "JetBrains", "https://www.jetbrains.com/favicon.ico", "global"},
			{"vscode-marketplace", "VS Code Marketplace", "https://marketplace.visualstudio.com/favicon.ico", "global"},
			{"docker-registry", "Docker Registry", "https://registry-1.docker.io/v2/", "global"},
			{"ghcr", "GHCR", "https://ghcr.io/v2/", "global"},
			{"gcr", "Google Container Registry", "https://gcr.io/v2/", "global"},
			{"k8s-registry", "Kubernetes Registry", "https://registry.k8s.io/v2/", "global"},
			{"github-raw", "GitHub Raw", "https://raw.githubusercontent.com/robots.txt", "global"},
			{"quay", "Quay", "https://quay.io/robots.txt", "global"},
			{"rubygems", "RubyGems", "https://rubygems.org/favicon.ico", "global"},
			{"nuget", "NuGet", "https://api.nuget.org/v3/index.json", "global"},
			{"packagist", "Packagist", "https://packagist.org/favicon.ico", "global"},
			{"gradle", "Gradle Plugins", "https://plugins.gradle.org/m2/", "global"},
			{"jsr", "JSR", "https://jsr.io/favicon.ico", "global"},
			{"deno", "Deno", "https://deno.land/favicon.ico", "global"},
			{"codeberg", "Codeberg", "https://codeberg.org/favicon.ico", "global"},
			{"hackernews", "Hacker News", "https://news.ycombinator.com/favicon.ico", "global"},
			{"devto", "DEV", "https://dev.to/favicon.ico", "global"},
			{"mdn", "MDN", "https://developer.mozilla.org/favicon.ico", "global"},
			{"postman", "Postman", "https://www.postman.com/favicon.ico", "global"},
			{"sentry", "Sentry", "https://sentry.io/favicon.ico", "global"},
			{"grafana", "Grafana", "https://grafana.com/favicon.ico", "global"},
			{"terraform", "Terraform", "https://registry.terraform.io/favicon.ico", "global"},
		},
	},
	{
		ID: "cloud", Emoji: "☁️",
		Sites: []Site{
			{"cloudflare", "Cloudflare", "https://speed.cloudflare.com/favicon.ico", "global"},
			{"aws", "AWS", "https://aws.amazon.com/favicon.ico", "global"},
			{"azure", "Azure", "https://azure.microsoft.com/favicon.ico", "global"},
			{"gcloud", "Google Cloud", "https://cloud.google.com/favicon.ico", "global"},
			{"vercel", "Vercel", "https://vercel.com/favicon.ico", "global"},
			{"netlify", "Netlify", "https://www.netlify.com/favicon.ico", "global"},
			{"digitalocean", "DigitalOcean", "https://www.digitalocean.com/favicon.ico", "global"},
			{"oraclecloud", "Oracle Cloud", "https://www.oracle.com/favicon.ico", "global"},
			{"akamai", "Akamai", "https://www.akamai.com/favicon.ico", "global"},
			{"fastly", "Fastly", "https://www.fastly.com/favicon.ico", "global"},
			{"jsdelivr", "jsDelivr", "https://cdn.jsdelivr.net/favicon.ico", "global"},
			{"unpkg", "unpkg", "https://unpkg.com/favicon.ico", "global"},
			{"heroku", "Heroku", "https://www.heroku.com/favicon.ico", "global"},
			{"firebase", "Firebase", "https://firebase.google.com/favicon.ico", "global"},
			{"supabase", "Supabase", "https://supabase.com/robots.txt", "global"},
			{"railway", "Railway", "https://railway.com/favicon.ico", "global"},
			{"flyio", "Fly.io", "https://fly.io/favicon.ico", "global"},
			{"render", "Render", "https://render.com/robots.txt", "global"},
			{"hetzner", "Hetzner", "https://www.hetzner.com/favicon.ico", "global"},
			{"linode", "Linode", "https://www.linode.com/favicon.ico", "global"},
			{"ibmcloud", "IBM Cloud", "https://cloud.ibm.com/favicon.ico", "global"},
			{"alibabacloud", "Alibaba Cloud", "https://www.alibabacloud.com/favicon.ico", "global"},
			{"tencentcloud", "Tencent Cloud", "https://www.tencentcloud.com/favicon.ico", "global"},
			{"bunny", "Bunny.net", "https://bunny.net/favicon.ico", "global"},
		},
	},
	{
		ID: "finance", Emoji: "💳",
		Sites: []Site{
			{"paypal", "PayPal", "https://www.paypal.com/favicon.ico", "global"},
			{"wise", "Wise", "https://wise.com/robots.txt", "global"},
			{"revolut", "Revolut", "https://www.revolut.com/robots.txt", "global"},
			{"stripe", "Stripe", "https://stripe.com/favicon.ico", "global"},
			{"visa", "Visa", "https://www.visa.com/robots.txt", "global"},
			{"mastercard", "Mastercard", "https://developer.mastercard.com/favicon.ico", "global"},
			{"western-union", "Western Union", "https://www.westernunion.com/robots.txt", "global"},
			{"payoneer", "Payoneer", "https://www.payoneer.com/robots.txt", "global"},
			{"klarna", "Klarna", "https://docs.klarna.com/favicon.ico", "global"},
			{"monzo", "Monzo", "https://monzo.com/robots.txt", "global"},
			{"tradingview", "TradingView", "https://www.tradingview.com/favicon.ico", "global"},
			{"yahoo-finance", "Yahoo Finance", "https://finance.yahoo.com/favicon.ico", "global"},
			{"robinhood", "Robinhood", "https://robinhood.com/favicon.ico", "global"},
			{"ibkr", "Interactive Brokers", "https://www.interactivebrokers.com/favicon.ico", "global"},
			{"schwab", "Charles Schwab", "https://www.schwab.com/favicon.ico", "global"},
			{"moomoo", "moomoo", "https://www.moomoo.com/favicon.ico", "global"},
			{"cashapp", "Cash App", "https://cash.app/favicon.ico", "global"},
			{"venmo", "Venmo", "https://venmo.com/favicon.ico", "global"},
		},
	},
	{
		ID: "crypto", Emoji: "💰",
		Sites: []Site{
			{"binance", "Binance", "https://www.binance.com/favicon.ico", "global"},
			{"coinbase", "Coinbase", "https://www.coinbase.com/favicon.ico", "global"},
			{"okx", "OKX", "https://www.okx.com/favicon.ico", "global"},
			{"kraken", "Kraken", "https://www.kraken.com/favicon.ico", "global"},
			{"bybit", "Bybit", "https://www.bybit.com/favicon.ico", "global"},
			{"kucoin", "KuCoin", "https://www.kucoin.com/favicon.ico", "global"},
			{"gate", "Gate", "https://www.gate.io/favicon.ico", "global"},
			{"bitget", "Bitget", "https://www.bitget.com/favicon.ico", "global"},
			{"htx", "HTX", "https://www.htx.com/favicon.ico", "global"},
			{"mexc", "MEXC", "https://www.mexc.com/favicon.ico", "global"},
			{"coinmarketcap", "CoinMarketCap", "https://coinmarketcap.com/favicon.ico", "global"},
			{"coingecko", "CoinGecko", "https://www.coingecko.com/favicon.ico", "global"},
			{"cryptocom", "Crypto.com", "https://crypto.com/favicon.ico", "global"},
			{"etherscan", "Etherscan", "https://etherscan.io/favicon.ico", "global"},
			{"metamask", "MetaMask", "https://metamask.io/favicon.ico", "global"},
			{"uniswap", "Uniswap", "https://app.uniswap.org/favicon.ico", "global"},
			{"defillama", "DefiLlama", "https://defillama.com/favicon.ico", "global"},
			{"bitstamp", "Bitstamp", "https://www.bitstamp.net/favicon.ico", "global"},
		},
	},
	{
		ID: "ecommerce", Emoji: "🛒",
		Sites: []Site{
			{"amazon", "Amazon", "https://www.amazon.com/favicon.ico", "global"},
			{"aliexpress", "AliExpress", "https://www.aliexpress.com/favicon.ico", "global"},
			{"temu", "Temu", "https://www.temu.com/favicon.ico", "global"},
			{"shein", "SHEIN", "https://www.shein.com/favicon.ico", "global"},
			{"alibaba", "Alibaba.com", "https://www.alibaba.com/favicon.ico", "global"},
			{"ebay", "eBay", "https://www.ebay.com/favicon.ico", "global"},
			{"etsy", "Etsy", "https://www.etsy.com/favicon.ico", "global"},
			{"shopify", "Shopify", "https://www.shopify.com/favicon.ico", "global"},
			{"walmart", "Walmart", "https://www.walmart.com/favicon.ico", "global"},
			{"shopee", "Shopee", "https://shopee.com/favicon.ico", "global"},
			{"lazada", "Lazada", "https://www.lazada.sg/favicon.ico", "global"},
			{"mercadolibre", "Mercado Libre", "https://www.mercadolibre.com/favicon.ico", "global"},
			{"rakuten", "Rakuten", "https://www.rakuten.co.jp/favicon.ico", "global"},
			{"ikea", "IKEA", "https://www.ikea.com/favicon.ico", "global"},
			{"zalando", "Zalando", "https://www.zalando.com/favicon.ico", "global"},
			{"newegg", "Newegg", "https://www.newegg.com/favicon.ico", "global"},
			{"dhgate", "DHgate", "https://www.dhgate.com/favicon.ico", "global"},
			{"vinted", "Vinted", "https://www.vinted.com/favicon.ico", "global"},
			{"asos", "ASOS", "https://www.asos.com/favicon.ico", "global"},
			{"nike", "Nike", "https://www.nike.com/favicon.ico", "global"},
			{"adidas", "Adidas", "https://www.adidas.com/favicon.ico", "global"},
			{"wayfair", "Wayfair", "https://www.wayfair.com/favicon.ico", "global"},
		},
	},
	{
		ID: "news", Emoji: "📰",
		Sites: []Site{
			{"bbc", "BBC", "https://www.bbc.com/favicon.ico", "global"},
			{"reuters", "Reuters", "https://www.reuters.com/favicon.ico", "global"},
			{"apnews", "AP News", "https://apnews.com/favicon.ico", "global"},
			{"cnn", "CNN", "https://edition.cnn.com/favicon.ico", "global"},
			{"nytimes", "The New York Times", "https://www.nytimes.com/favicon.ico", "global"},
			{"guardian", "The Guardian", "https://www.theguardian.com/favicon.ico", "global"},
			{"dw", "DW", "https://www.dw.com/favicon.ico", "global"},
			{"aljazeera", "Al Jazeera", "https://www.aljazeera.com/favicon.ico", "global"},
			{"france24", "France 24", "https://www.france24.com/favicon.ico", "global"},
			{"bloomberg", "Bloomberg", "https://www.bloomberg.com/favicon.ico", "global"},
			{"wikipedia", "Wikipedia", "https://www.wikipedia.org/favicon.ico", "global"},
			{"economist", "The Economist", "https://www.economist.com/favicon.ico", "global"},
			{"cnbc", "CNBC", "https://www.cnbc.com/favicon.ico", "global"},
			{"forbes", "Forbes", "https://www.forbes.com/favicon.ico", "global"},
			{"nyt-cn", "NYT 中文网", "https://cn.nytimes.com/favicon.ico", "global"},
			{"bbc-zh", "BBC 中文", "https://www.bbc.co.uk/zhongwen/simp", "global"},
			{"voa-zh", "VOA 中文", "https://www.voachinese.com/favicon.ico", "global"},
			{"initium", "端传媒", "https://theinitium.com/favicon.ico", "global"},
			{"zaobao", "联合早报", "https://www.zaobao.com/favicon.ico", "global"},
			{"npr", "NPR", "https://www.npr.org/favicon.ico", "global"},
			{"wsj", "The Wall Street Journal", "https://www.wsj.com/favicon.ico", "global"},
			{"washingtonpost", "The Washington Post", "https://www.washingtonpost.com/favicon.ico", "global"},
			{"ft", "Financial Times", "https://www.ft.com/robots.txt", "global"},
			{"nbcnews", "NBC News", "https://www.nbcnews.com/robots.txt", "global"},
			{"foxnews", "Fox News", "https://www.foxnews.com/robots.txt", "global"},
		},
	},
}

// Site categories, as the page shows them: a few broad kinds of site, the
// themes of the preset groups folded into them; mainland China's sites;
// the country groups under one "region" category, each a region of it;
// and the sites of no group.
const (
	// CategoryGeneral is the user's favorites (常用), which the page shows
	// before the categories: a list of sites of any category, never the
	// category of a site.
	CategoryGeneral  = "general"
	CategoryDomestic = "domestic" // sites in mainland China
	CategoryRegion   = "region"   // what each country reaches at home
	CategoryOther    = "other"    // sites of no preset group
)

// categoryThemes are the categories and the preset themes each holds, in
// the order the page shows them: what most people test first leads.
var categoryThemes = []struct {
	id     string
	themes []string
}{
	{"web", []string{"web"}},
	{"ai", []string{"ai"}},
	{"media", []string{"streaming", "music"}},
	{"social", []string{"social"}},
	{"cloud", []string{"cloud"}},
	{"games", []string{"gaming"}},
	{"dev", []string{"developer"}},
	{"work", []string{"productivity", "education"}},
	{"money", []string{"finance", "crypto"}},
	{"shop", []string{"ecommerce"}},
	{"news", []string{"news"}},
	{"travel", []string{"travel"}},
	{"sports", []string{"sports"}},
}

// categoryOrder is every category, in the order the page shows them.
var categoryOrder = func() []string {
	var out []string
	for _, c := range categoryThemes {
		out = append(out, c.id)
	}
	return append(out, CategoryRegion, CategoryOther)
}()

// regionCodes are the country groups' countries, for their flags and names.
var regionCodes = map[string]string{
	"iran": "IR", "russia": "RU", "brazil": "BR", "india": "IN", "nigeria": "NG", "germany": "DE",
	"albania": "AL", "kosovo": "XK", "thailand": "TH", "mexico": "MX", "china": "CN",
	"japan": "JP", "korea": "KR", "hongkong": "HK", "taiwan": "TW", "singapore": "SG", "usa": "US", "uk": "GB",
}

// DefaultFavoriteSiteIDs are the favorites (常用) of a new installation:
// a start, which the user may change as any other.
var DefaultFavoriteSiteIDs = []string{"google", "apple", "github", "cloudflare", "wikipedia"}

// categoryOf and regionOf are the category and, under region, the country
// group of every site of the preset groups; a site in several takes the
// first category in categoryOrder.
var categoryOf, regionOf = func() (map[string]string, map[string]string) {
	cats, regions := map[string]string{}, map[string]string{}
	byID := map[string]SitePreset{}
	for _, p := range SitePresets {
		byID[p.ID] = p
	}
	for _, c := range categoryThemes {
		for _, t := range c.themes {
			for _, s := range byID[t].Sites {
				if _, ok := cats[s.ID]; !ok {
					cats[s.ID] = c.id
				}
			}
		}
	}
	for _, p := range SitePresets {
		for _, s := range p.Sites {
			if s.Group == "domestic" {
				cats[s.ID], regions[s.ID] = CategoryRegion, "china"
			}
		}
	}
	for _, p := range SitePresets {
		if _, ok := regionCodes[p.ID]; !ok {
			continue
		}
		for _, s := range p.Sites {
			if _, ok := cats[s.ID]; !ok {
				cats[s.ID], regions[s.ID] = CategoryRegion, p.ID
			}
		}
	}
	return cats, regions
}()

// SiteCategory is the category the connectivity page shows a site under:
// the kind of site, else the country region, else other. Mainland China's
// sites use the china region.
func SiteCategory(s Site) string {
	if c, ok := categoryOf[s.ID]; ok {
		return c
	}
	return CategoryOther
}

// SiteRegion is the country group of a site of the region category, ""
// for any other.
func SiteRegion(s Site) string {
	if SiteCategory(s) != CategoryRegion {
		return ""
	}
	return regionOf[s.ID]
}

// RegionCode is the country of a country group, as ISO 3166 letters.
func RegionCode(region string) string { return regionCodes[region] }

// SiteCategories are the categories of sites, in the order the page shows
// them.
func SiteCategories(sites []Site) []string {
	var out []string
	for _, c := range categoryOrder {
		if slices.ContainsFunc(sites, func(s Site) bool { return SiteCategory(s) == c }) {
			out = append(out, c)
		}
	}
	return out
}

// SiteRegions are the country groups the sites of the region category
// come from, in the order of the preset groups.
func SiteRegions(sites []Site) []string {
	var out []string
	for _, p := range SitePresets {
		if _, ok := regionCodes[p.ID]; ok && slices.ContainsFunc(sites, func(s Site) bool { return SiteRegion(s) == p.ID }) {
			out = append(out, p.ID)
		}
	}
	return out
}

// defaultSites are every site of the preset groups, by category, each
// once, at the address that tests it best.
func defaultSites() []Site {
	better := map[string]Site{
		"cloudflare": {"cloudflare", "Cloudflare", "https://www.cloudflare.com/cdn-cgi/trace", "global"},
	}
	var all []Site
	seen := map[string]bool{}
	add := func(s Site) {
		if seen[s.ID] {
			return
		}
		seen[s.ID] = true
		if b, ok := better[s.ID]; ok {
			s = b
		}
		all = append(all, s)
	}
	var pool []Site
	for _, p := range SitePresets {
		pool = append(pool, p.Sites...)
	}
	for _, c := range categoryOrder {
		for _, s := range pool {
			if SiteCategory(s) == c {
				add(s)
			}
		}
	}
	return all
}

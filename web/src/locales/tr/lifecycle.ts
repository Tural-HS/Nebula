// Turkish strings for archiving, restoring and deleting projects and workspaces (ADR-0039).
import type { Messages } from "../../i18n/locale";

export const tlc = {
  dangerZone: "Tehlikeli bölge",
  cancel: "İptal",
  restore: "Geri yükle",
  open: "Aç",
  archivedOn: (date: string) => `Arşivlendi: ${date}`,
  workspace: {
    archiveTitle: "Çalışma alanını arşivle",
    archiveHint:
      "Çalışma alanı salt okunur olur ve ana sayfadan ve çalışma alanı listesinden gizlenir. Otomasyonlar, webhook'lar, yinelenen görevler, hatırlatmalar ve e-postalar durur; formlar talep kabul etmez. İstediğiniz zaman geri yükleyebilirsiniz.",
    archive: "Arşivle",
    archiveConfirm: (name: string) => `“${name}” arşivlensin mi?`,
    archiveConfirmHint: "Üyeler verileri görmeye devam eder ama hiçbir şeyi değiştiremez. Geri yüklenene kadar çalışma alanı listelerde görünmez.",
    archived: "Çalışma alanı arşivlendi.",
    restored: "Çalışma alanı geri yüklendi.",
    ownerOnly: "Çalışma alanını yalnızca sahibi arşivleyebilir ve silebilir.",
    banner: "Bu çalışma alanı arşivlendi: verilere bakabilirsiniz ama değiştiremezsiniz.",
    bannerAdmin: "Bu çalışma alanı arşivlendi: verilere bakabilirsiniz ama değiştiremezsiniz. Yeniden çalışmak için geri yükleyin.",
    deleteTitle: "Çalışma alanını kalıcı olarak sil",
    deleteHint:
      "Tüm projeler, görevler, yorumlar, dosyalar, formlar, ayarlar ve denetim günlüğü geri alınamaz şekilde silinir. Silme kaydı yalnızca sunucu günlüğünde kalır.",
    deleteNeedsArchive: "Silmek için önce çalışma alanını arşivleyin.",
    deleteConfirm: (name: string) => `“${name}” kalıcı olarak silinsin mi?`,
    evidenceTitle: "Önce denetim kanıt paketini indirin",
    evidenceHint:
      "Mali kayıtları saklama yükümlülüğünüz olabilir. Silindikten sonra geçmişi, onayları ve dosyaları kimse geri getiremez.",
    evidenceLink: "Kanıt paketini indir (Ayarlar → Denetim günlüğü)",
    evidenceAck: "Kanıt paketini indirdim veya gerekmediğinden eminim",
    typeSlug: (slug: string) => `Onaylamak için çalışma alanının kısa adını yazın: ${slug}`,
    deleteButton: "Kalıcı olarak sil",
    deleted: "Çalışma alanı silindi.",
    deleting: "Çalışma alanı siliniyor. Büyük bir çalışma alanı birkaç dakika sürebilir.",
  },
  project: {
    archiveTitle: "Projeyi arşivle",
    archiveHint:
      "Proje listelerden, aramadan, ana sayfadan ve kuyruklardan gizlenir; formları talep kabul etmez; yinelenen görevler, otomasyonlar ve webhook'lar durur. Bir yönetici istediği zaman geri yükleyebilir.",
    archive: "Arşivle",
    archiveConfirm: (name: string) => `“${name}” projesi arşivlensin mi?`,
    archived: "Proje arşivlendi.",
    restored: "Proje geri yüklendi.",
    deleteHint: "Projeyi kalıcı olarak silmek için önce arşivleyin. Arşivlenmiş projeyi yalnızca çalışma alanı sahibi silebilir.",
    deleteTitle: "Projeyi kalıcı olarak sil",
    deleteConsequences:
      "Tüm görevler, alt görevler, yorumlar, dosyalar, bağlantılar, onaylar, görünümler, formlar ve yinelenen görevler geri alınamaz şekilde silinir. Denetim günlüğünde silme kaydı kalır.",
    deleteConfirm: (name: string) => `“${name}” projesi kalıcı olarak silinsin mi?`,
    typeKey: (key: string) => `Onaylamak için proje anahtarını yazın: ${key}`,
    deleteButton: "Kalıcı olarak sil",
    deleted: "Proje silindi.",
    deleting: "Proje siliniyor. Büyük bir proje birkaç dakika sürebilir.",
    stateTitle: "Bu proje arşivlendi",
    stateHint: "Arşivlenmiş projenin görevleri görünmez ve değiştirilemez.",
    stateAskAdmin: "Projeye yeniden ihtiyacınız varsa bir çalışma alanı yöneticisine başvurun.",
    deletingTitle: "Bu proje siliniyor",
    archivedList: "Arşivlenmiş projeler",
    archivedListHint: "Yalnızca yöneticiler görür. Açıp geri yükleyebilir veya (sahip) kalıcı olarak silebilirsiniz.",
  },
  home: {
    title: "Arşivlenmiş çalışma alanları",
    hint: "Yalnızca sahipler ve yöneticiler görür. Geri yüklenene kadar salt okunurdur.",
  },
  formClosedArchived: "Bu form arşivlenmiş bir projeye ait ve talep kabul etmiyor.",
} satisfies Messages["lifecycle"];

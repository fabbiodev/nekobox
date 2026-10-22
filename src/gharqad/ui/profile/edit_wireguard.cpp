



#include <nekobox/ui/profile/edit_wireguard.h>

#include <nekobox/configs/proxy/WireguardBean.h>
#include <QFormLayout>
#include <QGridLayout>
#include <QMessageBox>

EditWireguard::EditWireguard(QWidget *parent) : QWidget(parent), ui(new Ui::EditWireguard) {
    ui->setupUi(this);
    auto advanced = new QGroupBox(QStringLiteral("AmneziaWG 3.1"), ui->enable_amnezia);
    auto form = new QFormLayout(advanced);
    const QList<QPair<QString, QString>> fields = {
        {"header_protection_key", "HeaderProtectionKey"},
        {"content_padding_addition", "ContentPaddingAddition"},
        {"rekey_after_time", "RekeyAfterTime"}, {"rekey_timeout", "RekeyTimeout"},
        {"reject_after_time", "RejectAfterTime"}, {"keepalive_timeout", "KeepaliveTimeout"},
        {"max_handshake_attempts", "MaxHandshakeAttempts"}
    };
    for (const auto &field : fields) {
        auto input = new QLineEdit(advanced);
        input->setObjectName(field.first);
        if (field.first == "header_protection_key") input->setEchoMode(QLineEdit::PasswordEchoOnEdit);
        else input->setPlaceholderText(QStringLiteral("0 / 10-100"));
        awg31Fields.insert(field.first, input);
        form->addRow(field.second, input);
    }
    randomTrailers = new QCheckBox(QStringLiteral("RandomTrailers"), advanced);
    disableCookies = new QCheckBox(QStringLiteral("DisableCookies"), advanced);
    form->addRow(randomTrailers, disableCookies);
    auto grid = qobject_cast<QGridLayout *>(ui->enable_amnezia->layout());
    grid->addWidget(advanced, grid->rowCount(), 0, 1, grid->columnCount());
}

EditWireguard::~EditWireguard() {
    delete ui;
}

void EditWireguard::onStart(std::shared_ptr<Configs::ProxyEntity> ent) {
    this->ent = ent;
    auto bean = ent->WireguardBean();

#ifndef Q_OS_UNIX
    adjustSize();
#endif

    ui->private_key->setText(bean->privateKey);
    ui->public_key->setText(bean->publicKey);
    ui->preshared_key->setText(bean->preSharedKey);

    auto reservedStr = bean->FormatReserved().replace("-", ",");
    ui->reserved->setText(reservedStr);
    ui->persistent_keepalive->setText(bean->persistentKeepaliveRange.isEmpty()
        ? QString::number(bean->persistentKeepalive) : bean->persistentKeepaliveRange);
    ui->mtu->setText(QString::number(bean->MTU));
    ui->sys_ifc->setChecked(bean->useSystemInterface);
    ui->local_addr->setText(bean->localAddress.join(","));

    ui->enable_amnezia->setChecked(ent->type == "awg");

    P_LOAD_INT(junk_packet_count);
    P_LOAD_INT(junk_packet_min_size);
    P_LOAD_INT(junk_packet_max_size);

    P_LOAD_INT(init_packet_junk_size);
    P_LOAD_INT(response_packet_junk_size);
    P_LOAD_INT(cookie_reply_junk_size);
    P_LOAD_INT(transport_packet_junk_size);

    P_LOAD_STRING(init_packet_magic_header);
    P_LOAD_STRING(response_packet_magic_header);
    P_LOAD_STRING(cookie_reply_magic_header);
    P_LOAD_STRING(transport_packet_magic_header);

    P_LOAD_STRING(i1);
    P_LOAD_STRING(i2);
    P_LOAD_STRING(i3);
    P_LOAD_STRING(i4);
    P_LOAD_STRING(i5);
    awg31Fields["header_protection_key"]->setText(bean->header_protection_key);
    awg31Fields["content_padding_addition"]->setText(bean->content_padding_addition);
    awg31Fields["rekey_after_time"]->setText(bean->rekey_after_time);
    awg31Fields["rekey_timeout"]->setText(bean->rekey_timeout);
    awg31Fields["reject_after_time"]->setText(bean->reject_after_time);
    awg31Fields["keepalive_timeout"]->setText(bean->keepalive_timeout);
    awg31Fields["max_handshake_attempts"]->setText(bean->max_handshake_attempts);
    randomTrailers->setChecked(bean->random_trailers);
    disableCookies->setChecked(bean->disable_cookies);
}

bool EditWireguard::onEnd() {
    auto bean = ent->unlock(ent->WireguardBean());

    bean->privateKey = ui->private_key->text();
    bean->publicKey = ui->public_key->text();
    bean->preSharedKey = ui->preshared_key->text();
    auto rawReserved = ui->reserved->text();
    bean->reserved = {};
    for (const auto& item: rawReserved.split(",")) {
        if (item.trimmed().isEmpty()) continue;
        bean->reserved += item.trimmed().toInt();
    }
    bean->persistentKeepalive = ui->persistent_keepalive->text().toInt();
    bean->persistentKeepaliveRange = ui->persistent_keepalive->text().contains('-')
        ? ui->persistent_keepalive->text().trimmed() : QString();
    bean->MTU = ui->mtu->text().toInt();
    bean->useSystemInterface = ui->sys_ifc->isChecked();
    bean->localAddress = ui->local_addr->text().replace(" ", "").split(",");

    bean->enableAmnezia(ui->enable_amnezia->isChecked());

    P_SAVE_INT(junk_packet_count);
    P_SAVE_INT(junk_packet_min_size);
    P_SAVE_INT(junk_packet_max_size);

    P_SAVE_INT(init_packet_junk_size);
    P_SAVE_INT(response_packet_junk_size);
    P_SAVE_INT(cookie_reply_junk_size);
    P_SAVE_INT(transport_packet_junk_size);

    P_SAVE_STRING(init_packet_magic_header);
    P_SAVE_STRING(response_packet_magic_header);
    P_SAVE_STRING(cookie_reply_magic_header);
    P_SAVE_STRING(transport_packet_magic_header);

    P_SAVE_STRING(i1);
    P_SAVE_STRING(i2);
    P_SAVE_STRING(i3);
    P_SAVE_STRING(i4);
    P_SAVE_STRING(i5);
    bean->header_protection_key = awg31Fields["header_protection_key"]->text().trimmed();
    bean->content_padding_addition = awg31Fields["content_padding_addition"]->text().trimmed();
    bean->rekey_after_time = awg31Fields["rekey_after_time"]->text().trimmed();
    bean->rekey_timeout = awg31Fields["rekey_timeout"]->text().trimmed();
    bean->reject_after_time = awg31Fields["reject_after_time"]->text().trimmed();
    bean->keepalive_timeout = awg31Fields["keepalive_timeout"]->text().trimmed();
    bean->max_handshake_attempts = awg31Fields["max_handshake_attempts"]->text().trimmed();
    bean->random_trailers = randomTrailers->isChecked();
    bean->disable_cookies = disableCookies->isChecked();
    if (ui->enable_amnezia->isChecked() && !bean->ValidateAmnezia()) {
        QMessageBox::warning(this, tr("AmneziaWG"),
            tr("Invalid AmneziaWG parameters: check ranges, keys and packet padding."));
        return false;
    }

    return true;
}

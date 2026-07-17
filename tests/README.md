# Tests

Entegrasyon testleri Linux VM içinde çalıştırılmalıdır. Host filesystem’e yönelik destructive testler yalnızca disposable VM veya açıkça oluşturulmuş test image’ında yapılır.

Her rollback testinden önce lower layer için hash/metadata manifest’i çıkarılır ve rollback sonrasında karşılaştırılır.

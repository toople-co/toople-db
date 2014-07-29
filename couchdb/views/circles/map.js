function(doc) {
	if (doc.type == 'member') {
		emit(doc.user, {_id: doc.circle});
	}
}